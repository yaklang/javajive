package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxInputBytes     = int64(256 << 20)
	maxOutputJarBytes = int64(256 << 20)
	defaultOutputCap  = int64(1 << 20)
	defaultTextCap    = int64(64 << 10)
)

type oracleIdentity struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Family        string `json:"family"`
	SourceCommit  string `json:"source_commit"`
	ArchiveSHA256 string `json:"archive_sha256"`
}

type inputIdentity struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	abs    string
}

type streamRecord struct {
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text"`
}

type archiveArtifact struct {
	Present bool     `json:"present"`
	Name    string   `json:"name,omitempty"`
	SHA256  string   `json:"sha256,omitempty"`
	Size    int64    `json:"size,omitempty"`
	Entries []string `json:"entries,omitempty"`
	Error   string   `json:"inventory_error,omitempty"`
}

type classMember struct {
	Name       string `json:"name"`
	Descriptor string `json:"descriptor"`
}

type classInventory struct {
	Entry   string        `json:"entry"`
	Name    string        `json:"name"`
	Fields  []classMember `json:"fields"`
	Methods []classMember `json:"methods"`
}

type jarArtifact struct {
	Present bool             `json:"present"`
	SHA256  string           `json:"sha256,omitempty"`
	Size    int64            `json:"size,omitempty"`
	Entries []string         `json:"entries,omitempty"`
	Classes []classInventory `json:"classes,omitempty"`
	Error   string           `json:"inventory_error,omitempty"`
}

type probeResult struct {
	OracleID            string            `json:"oracle_id"`
	OracleVersion       string            `json:"oracle_version"`
	OracleArchiveSHA256 string            `json:"oracle_archive_sha256"`
	InputPath           string            `json:"input_path"`
	InputSHA256         string            `json:"input_sha256"`
	ExitCode            int               `json:"exit_code"`
	TimedOut            bool              `json:"timed_out"`
	OutputLimited       bool              `json:"output_limited"`
	ConversionRuntimeMS int64             `json:"conversion_runtime_ms"`
	Stdout              streamRecord      `json:"stdout"`
	Stderr              streamRecord      `json:"stderr"`
	OutputJar           jarArtifact       `json:"output_jar"`
	ErrorZips           []archiveArtifact `json:"error_zips"`
	StartError          string            `json:"start_error,omitempty"`
}

type probeReport struct {
	SchemaVersion  int              `json:"schema_version"`
	ManifestSHA256 string           `json:"manifest_sha256"`
	Oracles        []oracleIdentity `json:"oracles"`
	Inputs         []inputIdentity  `json:"inputs"`
	Results        []probeResult    `json:"results"`
}

func runProbe(args []string) error {
	fs := newFlagSet("probe")
	cache := fs.String("cache", defaultCacheDir(), "cache directory")
	manifestPath := fs.String("manifest", "", "alternate manifest")
	versions := fs.String("versions", "all", "comma-separated ids or versions")
	output := fs.String("output", "-", "JSON output file or -")
	timeout := fs.Duration("timeout", 60*time.Second, "per-conversion timeout")
	outputCap := fs.Int64("output-cap", defaultOutputCap, "combined stdout/stderr byte cap")
	java := fs.String("java", "java", "Java executable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("probe requires at least one DEX or APK input")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return fmt.Errorf("timeout must be between 1ns and 10m")
	}
	if *outputCap < 1024 || *outputCap > 16<<20 {
		return fmt.Errorf("output-cap must be between 1024 and 16777216")
	}
	m, manifestHash, err := loadManifest(*manifestPath)
	if err != nil {
		return err
	}
	oracles, err := selectOracles(m, *versions)
	if err != nil {
		return err
	}
	inputs := make([]inputIdentity, 0, fs.NArg())
	for _, arg := range fs.Args() {
		input, err := identifyInput(arg)
		if err != nil {
			return err
		}
		inputs = append(inputs, input)
	}
	report := probeReport{SchemaVersion: 1, ManifestSHA256: manifestHash, Inputs: inputs}
	for _, oracle := range oracles {
		record, err := ensureOracle(context.Background(), *cache, oracle)
		if err != nil {
			return fmt.Errorf("prepare %s: %w", oracle.ID, err)
		}
		report.Oracles = append(report.Oracles, oracleIdentity{ID: oracle.ID, Version: oracle.Version, Family: oracle.Family, SourceCommit: oracle.SourceCommit, ArchiveSHA256: oracle.SHA256})
		for _, input := range inputs {
			result, err := probeOne(oracle, record.Distribution, input, *java, *timeout, *outputCap)
			if err != nil {
				return fmt.Errorf("probe %s on %s: %w", oracle.ID, input.Path, err)
			}
			report.Results = append(report.Results, result)
		}
	}
	for i := range report.Inputs {
		report.Inputs[i].abs = ""
	}
	return writeReportAtomic(*output, report)
}

func identifyInput(name string) (inputIdentity, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return inputIdentity{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return inputIdentity{}, fmt.Errorf("input %q: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxInputBytes {
		return inputIdentity{}, fmt.Errorf("input %q is not a regular file within the %d-byte limit", name, maxInputBytes)
	}
	hashValue, size, err := hashFile(abs, maxInputBytes)
	if err != nil {
		return inputIdentity{}, err
	}
	return inputIdentity{Path: filepath.Clean(name), SHA256: hashValue, Size: size, abs: abs}, nil
}

func probeOne(oracle Oracle, distribution string, input inputIdentity, java string, timeout time.Duration, outputCap int64) (probeResult, error) {
	jars, err := sortedJars(distribution)
	if err != nil {
		return probeResult{}, err
	}
	work, err := os.MkdirTemp("", "javajive-dex-oracle-probe-*")
	if err != nil {
		return probeResult{}, err
	}
	defer os.RemoveAll(work)
	stagedInput, err := stageInput(work, input)
	if err != nil {
		return probeResult{}, err
	}
	outputJar := filepath.Join(work, "output.jar")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	combined := &sharedOutputLimit{remaining: outputCap, cancel: cancel}
	stdout := newCapture(defaultTextCap, combined)
	stderr := newCapture(defaultTextCap, combined)
	cmd := exec.CommandContext(ctx, java,
		"-Xms32m", "-Xmx512m", "-XX:MaxMetaspaceSize=256m",
		"-Djava.awt.headless=true", "-Dfile.encoding=UTF-8",
		"-classpath", strings.Join(jars, string(os.PathListSeparator)),
		"com.googlecode.dex2jar.tools.Dex2jarCmd", "--force", "-o", filepath.Base(outputJar), filepath.Base(stagedInput),
	)
	cmd.Dir = work
	cmd.Env = cleanJavaEnv(os.Environ())
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	started := time.Now()
	runErr := cmd.Run()
	runtimeMS := time.Since(started).Milliseconds()
	result := probeResult{
		OracleID: oracle.ID, OracleVersion: oracle.Version, OracleArchiveSHA256: oracle.SHA256,
		InputPath: input.Path, InputSHA256: input.SHA256, ExitCode: exitCode(runErr),
		TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded), OutputLimited: combined.exceeded(),
		ConversionRuntimeMS: runtimeMS, Stdout: stdout.record(), Stderr: stderr.record(), ErrorZips: []archiveArtifact{},
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) && ctx.Err() == nil {
			result.StartError = runErr.Error()
		}
	}
	result.OutputJar = inventoryJar(outputJar)
	matches, err := filepath.Glob(filepath.Join(work, "*.zip"))
	if err != nil {
		return probeResult{}, err
	}
	sort.Strings(matches)
	for _, filename := range matches {
		result.ErrorZips = append(result.ErrorZips, inventoryZip(filename))
	}
	return result, nil
}

func cleanJavaEnv(env []string) []string {
	blocked := map[string]bool{"JAVA_TOOL_OPTIONS": true, "JDK_JAVA_OPTIONS": true, "_JAVA_OPTIONS": true}
	result := make([]string, 0, len(env))
	for _, value := range env {
		key, _, _ := strings.Cut(value, "=")
		if !blocked[key] {
			result = append(result, value)
		}
	}
	return result
}

func stageInput(work string, input inputIdentity) (string, error) {
	ext := strings.ToLower(filepath.Ext(input.abs))
	if ext != ".dex" && ext != ".apk" && ext != ".zip" {
		ext = ".bin"
	}
	destination := filepath.Join(work, "input"+ext)
	src, err := os.Open(input.abs)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, maxInputBytes+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		return "", errors.Join(copyErr, closeErr)
	}
	if n != input.Size || n > maxInputBytes || hex.EncodeToString(h.Sum(nil)) != input.SHA256 {
		return "", fmt.Errorf("input changed after hashing: %s", input.Path)
	}
	return destination, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type sharedOutputLimit struct {
	mu        sync.Mutex
	remaining int64
	limited   bool
	cancel    context.CancelFunc
}

func (l *sharedOutputLimit) account(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remaining -= int64(n)
	if l.remaining < 0 && !l.limited {
		l.limited = true
		l.cancel()
	}
}

func (l *sharedOutputLimit) exceeded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limited
}

type captureWriter struct {
	mu        sync.Mutex
	hash      hash.Hash
	text      []byte
	textLimit int64
	total     int64
	limit     *sharedOutputLimit
}

func newCapture(textLimit int64, limit *sharedOutputLimit) *captureWriter {
	return &captureWriter{hash: sha256.New(), textLimit: textLimit, limit: limit}
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.hash.Write(p)
	w.total += int64(len(p))
	if int64(len(w.text)) < w.textLimit {
		remaining := int(w.textLimit - int64(len(w.text)))
		if remaining > len(p) {
			remaining = len(p)
		}
		w.text = append(w.text, p[:remaining]...)
	}
	w.limit.account(len(p))
	return len(p), nil
}

func (w *captureWriter) record() streamRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	return streamRecord{SHA256: hex.EncodeToString(w.hash.Sum(nil)), Bytes: w.total, Truncated: w.total > int64(len(w.text)), Text: string(w.text)}
}

func hashFile(name string, limit int64) (string, int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, limit+1))
	if err != nil {
		return "", 0, err
	}
	if n > limit {
		return "", n, fmt.Errorf("file %q exceeds %d bytes", name, limit)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func writeReportAtomic(output string, report probeReport) error {
	if output == "-" {
		return writeJSON(os.Stdout, report)
	}
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".dex-oracle-result-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := writeJSON(tmp, report); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(output)
	}
	return os.Rename(tmpName, output)
}
