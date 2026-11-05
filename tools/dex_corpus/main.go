// Command dex_corpus fetches and probes the pinned adversarial DEX corpus.
// It translates bytes only; it never loads or executes generated or corpus code.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/dex2jar/report"
)

const (
	maxDownloadSize = int64(16 << 20)
	workerTimeout   = 10 * time.Second
)

type manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Artifacts     []artifact `json:"artifacts"`
}

type artifact struct {
	ID           string      `json:"id"`
	Path         string      `json:"path"`
	URL          *string     `json:"url"`
	Availability string      `json:"availability"`
	Size         *int64      `json:"size"`
	SHA256       *string     `json:"sha256"`
	Expected     expectation `json:"expected"`
}

type expectation struct {
	Disposition string `json:"disposition"`
	Stage       string `json:"stage"`
	Category    string `json:"category"`
}

type matrix struct {
	SchemaVersion int           `json:"schema_version"`
	Results       []probeResult `json:"results"`
}

type probeResult struct {
	ID          string             `json:"id"`
	Path        string             `json:"path"`
	Disposition string             `json:"disposition"`
	Stage       string             `json:"stage"`
	Category    string             `json:"category"`
	Context     *diagnosticContext `json:"context,omitempty"`
	Report      *dex2jar.Report    `json:"report,omitempty"`
	Error       string             `json:"error,omitempty"`
}

type diagnosticContext struct {
	Entry  string `json:"entry,omitempty"`
	Class  string `json:"class,omitempty"`
	Method string `json:"method,omitempty"`
	Offset int64  `json:"offset"`
	Op     string `json:"op,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "fetch":
		err = fetch(os.Args[2:])
	case "probe":
		err = probe(os.Args[2:])
	case "worker":
		err = worker(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dex_corpus <fetch|probe> -manifest manifest.json -cache DIR")
	os.Exit(2)
}

func commonFlags(name string, args []string) (*manifest, string, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	manifestPath := set.String("manifest", "dex2jar/testdata/adversarial/manifest.json", "corpus manifest")
	cache := set.String("cache", "", "corpus cache directory")
	if err := set.Parse(args); err != nil {
		return nil, "", err
	}
	if *cache == "" {
		return nil, "", errors.New("-cache is required")
	}
	data, err := os.ReadFile(*manifestPath)
	if err != nil {
		return nil, "", fmt.Errorf("read manifest: %w", err)
	}
	var m manifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&m); err != nil {
		return nil, "", fmt.Errorf("decode manifest: %w", err)
	}
	if m.SchemaVersion != 1 {
		return nil, "", fmt.Errorf("unsupported schema_version %d", m.SchemaVersion)
	}
	absCache, err := filepath.Abs(*cache)
	if err != nil {
		return nil, "", err
	}
	return &m, absCache, nil
}

func fetch(args []string) error {
	m, cache, err := commonFlags("fetch", args)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	for _, item := range m.Artifacts {
		if item.Availability != "available" {
			continue
		}
		destination, err := cachePath(cache, item.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", item.ID, err)
		}
		if err := verifyFile(destination, item); err == nil {
			fmt.Fprintf(os.Stderr, "%s: verified cached file\n", item.ID)
			continue
		}
		if item.URL == nil || item.Size == nil || item.SHA256 == nil {
			return fmt.Errorf("%s: incomplete download pin", item.ID)
		}
		if *item.Size > maxDownloadSize {
			return fmt.Errorf("%s: pinned size %d exceeds download bound %d", item.ID, *item.Size, maxDownloadSize)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		temp, err := os.CreateTemp(filepath.Dir(destination), ".dex-corpus-*")
		if err != nil {
			return err
		}
		tempName := temp.Name()
		ok := false
		defer func() {
			if !ok {
				_ = os.Remove(tempName)
			}
		}()
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, *item.URL, nil)
		if err != nil {
			_ = temp.Close()
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			_ = temp.Close()
			return fmt.Errorf("%s: download: %w", item.ID, err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			_ = temp.Close()
			return fmt.Errorf("%s: HTTP %s", item.ID, response.Status)
		}
		written, copyErr := io.Copy(temp, io.LimitReader(response.Body, maxDownloadSize+1))
		closeBodyErr := response.Body.Close()
		closeFileErr := temp.Close()
		if copyErr != nil {
			return fmt.Errorf("%s: download: %w", item.ID, copyErr)
		}
		if closeBodyErr != nil {
			return closeBodyErr
		}
		if closeFileErr != nil {
			return closeFileErr
		}
		if written > maxDownloadSize {
			return fmt.Errorf("%s: download exceeds bound", item.ID)
		}
		if err := verifyFile(tempName, item); err != nil {
			return fmt.Errorf("%s: %w", item.ID, err)
		}
		if err := os.Rename(tempName, destination); err != nil {
			return err
		}
		ok = true
		fmt.Fprintf(os.Stderr, "%s: fetched %d bytes\n", item.ID, written)
	}
	return nil
}

func probe(args []string) error {
	m, cache, err := commonFlags("probe", args)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	results := make([]probeResult, 0, len(m.Artifacts))
	for _, item := range m.Artifacts {
		if item.Availability != "available" {
			results = append(results, probeResult{ID: item.ID, Path: item.Path, Disposition: "pending", Stage: "availability", Category: "artifact-unavailable"})
			continue
		}
		path, err := cachePath(cache, item.Path)
		if err != nil {
			return err
		}
		if err := verifyFile(path, item); err != nil {
			return fmt.Errorf("%s: %w", item.ID, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), workerTimeout)
		command := exec.CommandContext(ctx, executable, "worker", "-path", path, "-id", item.ID, "-logical-path", item.Path)
		command.Stdin = nil
		output, commandErr := command.Output()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			results = append(results, probeResult{ID: item.ID, Path: item.Path, Disposition: "rejected", Stage: "timeout", Category: "timeout", Error: "probe exceeded 10s bound"})
			continue
		}
		if commandErr != nil {
			return fmt.Errorf("%s: worker: %w", item.ID, commandErr)
		}
		var result probeResult
		if err := json.Unmarshal(output, &result); err != nil {
			return fmt.Errorf("%s: worker JSON: %w", item.ID, err)
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(matrix{SchemaVersion: 1, Results: results})
}

func worker(args []string) error {
	set := flag.NewFlagSet("worker", flag.ContinueOnError)
	path := set.String("path", "", "verified input")
	id := set.String("id", "", "artifact ID")
	logicalPath := set.String("logical-path", "", "manifest path")
	if err := set.Parse(args); err != nil {
		return err
	}
	info, err := os.Stat(*path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDownloadSize {
		return errors.New("worker input violates file bounds")
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	jar, conversionReport, conversionErr := dex2jar.DexToJar(data, dex2jar.Options{MaxDEXSize: maxDownloadSize, MaxAPKSize: maxDownloadSize, MaxTotalDEXSize: maxDownloadSize, MaxDEXFiles: 32})
	result := probeResult{ID: *id, Path: *logicalPath}
	if conversionErr == nil {
		result.Disposition, result.Stage, result.Category = "accepted", "complete", "success"
		result.Report = &conversionReport
		_ = jar // Deliberately never write, load, inspect, or execute translated output.
	} else {
		result.Disposition = "rejected"
		result.Error = conversionErr.Error()
		result.Stage, result.Category = classify(conversionErr)
		if diagnostic, ok := report.AsDiagnostic(conversionErr); ok {
			result.Context = &diagnosticContext{Entry: diagnostic.Entry, Class: diagnostic.Class, Method: diagnostic.Method, Offset: diagnostic.Offset, Op: diagnostic.Op}
		}
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func cachePath(cache, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative || strings.HasPrefix(relative, "..") {
		return "", errors.New("unsafe manifest path")
	}
	path := filepath.Join(cache, filepath.FromSlash(relative))
	rel, err := filepath.Rel(cache, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("manifest path escapes cache")
	}
	return path, nil
}

func verifyFile(path string, item artifact) error {
	if item.Size == nil || item.SHA256 == nil {
		return errors.New("missing size/hash pin")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("cache entry is not a regular file")
	}
	if info.Size() != *item.Size {
		return fmt.Errorf("size %d, want %d", info.Size(), *item.Size)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, io.LimitReader(file, maxDownloadSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != *item.SHA256 {
		return fmt.Errorf("SHA-256 %s, want %s", actual, *item.SHA256)
	}
	return nil
}

func classify(err error) (string, string) {
	message := err.Error()
	stage := "translate"
	if diagnostic, ok := report.AsDiagnostic(err); ok {
		switch diagnostic.Op {
		case "header", "data section", "string_ids", "string_data", "type_ids", "proto_ids", "field_ids", "method_ids", "class_defs", "class_data", "code_item", "encoded_value", "APK", "APK entry", "input":
			stage = "parse"
		}
	} else if strings.Contains(message, "unsupported input magic") {
		return "identify", "magic"
	}
	switch {
	case strings.Contains(message, "unsupported DEX version"):
		return stage, "version"
	case strings.Contains(message, "annotations_off"):
		return stage, "annotations"
	case strings.Contains(message, "debug_info_off"):
		return stage, "debug-info"
	case strings.Contains(message, "endian_tag"):
		return stage, "endian"
	case strings.Contains(message, "file_size="):
		return stage, "file-size"
	case strings.Contains(message, "truncated"):
		return stage, "truncated"
	case strings.Contains(message, "range ") || strings.Contains(message, "overflows") || strings.Contains(message, "exceeds"):
		return stage, "bounds"
	case strings.Contains(message, "duplicate"):
		return stage, "duplicate"
	case strings.Contains(message, "unsupported opcode") || strings.Contains(message, "outside the supported"):
		return stage, "opcode"
	case strings.Contains(message, "unsupported"):
		return stage, "unsupported"
	default:
		return stage, "other"
	}
}
