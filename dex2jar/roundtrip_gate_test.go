package dex2jar_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar"
)

const (
	gateRuns          = 6
	gateProcessLimit  = 20 * time.Second
	gateProcessOutput = 1 << 20
)

type gateManifest struct {
	Schema   int           `json:"schema"`
	Fixtures []gateFixture `json:"fixtures"`
}

type gateFixture struct {
	File               string                         `json:"file"`
	SHA256             string                         `json:"sha256"`
	Size               int                            `json:"size"`
	InputInventory     []string                       `json:"input_inventory"`
	JarSHA256          string                         `json:"jar_sha256"`
	JarInventory       []string                       `json:"jar_inventory"`
	Report             gateReport                     `json:"report"`
	Classes            []string                       `json:"classes"`
	AllowedDiagnostics map[string]map[string][]string `json:"allowed_diagnostics"`
	Driver             string                         `json:"driver"`
	DriverClass        string                         `json:"driver_class"`
	Runs               []gateRun                      `json:"runs"`
	Quality            map[string]gateQuality         `json:"quality"`
}

type gateReport struct {
	DEXFiles          int `json:"dex_files"`
	Classes           int `json:"classes"`
	Methods           int `json:"methods"`
	TranslatedMethods int `json:"translated_methods"`
	Warnings          int `json:"warnings"`
}

type gateRun struct {
	Class  string `json:"class"`
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type gateQuality struct {
	Baseline gateMetrics `json:"baseline"`
	Maximum  gateMetrics `json:"maximum"`
}

type gateMetrics struct {
	Lines             int `json:"lines"`
	Tokens            int `json:"tokens"`
	Labels            int `json:"labels"`
	WhileTrue         int `json:"while_true"`
	SyntheticVars     int `json:"synthetic_vars"`
	UnsupportedThrows int `json:"unsupported_throws"`
}

type gateProcessResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

func TestStrictAcceptedDEXRoundtripGate(t *testing.T) {
	java := requiredTool(t, "java")
	javac := requiredTool(t, "javac")
	manifest := readGateManifest(t)
	assertGateFixtureInventory(t, manifest)
	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			runGateFixture(t, java, javac, fixture)
		})
	}
}

func runGateFixture(t *testing.T, java, javac string, fixture gateFixture) {
	t.Helper()
	root := t.TempDir()
	fixturePath := filepath.Join("testdata", "accept", fixture.File)
	input, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("fixture %q registered by manifest is unreadable: %v", fixture.File, err)
	}
	if len(input) != fixture.Size {
		t.Fatalf("fixture %q size changed: got %d bytes, manifest pins %d", fixture.File, len(input), fixture.Size)
	}
	if got := hashBytes(input); got != fixture.SHA256 {
		t.Fatalf("fixture %q SHA-256 changed: got %s, manifest pins %s", fixture.File, got, fixture.SHA256)
	}
	if got := inputInventory(t, fixture.File, input); !reflect.DeepEqual(got, fixture.InputInventory) {
		t.Fatalf("fixture %q input inventory changed: got %q, manifest pins %q", fixture.File, got, fixture.InputInventory)
	}

	var jar []byte
	var report dex2jar.Report
	for run := 0; run < gateRuns; run++ {
		gotJar, gotReport, err := dex2jar.DexToJar(input, dex2jar.Options{})
		if err != nil {
			t.Fatalf("fixture %q translation run %d/%d failed: %v", fixture.File, run+1, gateRuns, err)
		}
		if run == 0 {
			jar, report = gotJar, gotReport
			continue
		}
		if !bytes.Equal(gotJar, jar) {
			t.Fatalf("fixture %q translation is nondeterministic: run 1 SHA-256=%s, run %d SHA-256=%s", fixture.File, hashBytes(jar), run+1, hashBytes(gotJar))
		}
		if gotReport != report {
			t.Fatalf("fixture %q report is nondeterministic: run 1=%+v, run %d=%+v", fixture.File, report, run+1, gotReport)
		}
	}
	wantReport := dex2jar.Report{
		DEXFiles: fixture.Report.DEXFiles, Classes: fixture.Report.Classes,
		Methods: fixture.Report.Methods, TranslatedMethods: fixture.Report.TranslatedMethods,
		Warnings: fixture.Report.Warnings,
	}
	if report != wantReport {
		t.Fatalf("fixture %q translator report changed: got %+v, manifest pins %+v", fixture.File, report, wantReport)
	}
	if got := hashBytes(jar); got != fixture.JarSHA256 {
		t.Fatalf("fixture %q translated JAR SHA-256 changed: got %s, manifest pins %s", fixture.File, got, fixture.JarSHA256)
	}

	entries := readGateJar(t, fixture.File, jar)
	names := sortedKeys(entries)
	if !reflect.DeepEqual(names, fixture.JarInventory) {
		t.Fatalf("fixture %q translated JAR inventory changed: got %q, manifest pins %q", fixture.File, names, fixture.JarInventory)
	}
	classes := make(map[string][]byte)
	for name, data := range entries {
		if !strings.HasSuffix(name, ".class") {
			continue
		}
		parsed, err := classparser.Parse(data)
		if err != nil {
			t.Fatalf("fixture %q translated class %q does not parse: %v", fixture.File, name, err)
		}
		internalName := parsed.GetClassName()
		wantEntry := internalName + ".class"
		if wantEntry != name {
			t.Fatalf("fixture %q class identity mismatch: entry=%q parsed internal name=%q", fixture.File, name, internalName)
		}
		classes[internalName] = data
	}
	if got := dottedClassNames(classes); !reflect.DeepEqual(got, fixture.Classes) {
		t.Fatalf("fixture %q translated class identities changed: got %q, manifest pins %q", fixture.File, got, fixture.Classes)
	}

	translatedRoot := filepath.Join(root, "translated")
	writeClassTree(t, translatedRoot, classes)
	helperRoot := compileGateHelpers(t, javac, root)
	verifyGateClasses(t, java, helperRoot, translatedRoot, fixture.Classes, fixture.File+" translated")
	translatedRuns := runGatePrograms(t, java, javac, root, translatedRoot, fixture)

	for _, mode := range []classparser.DecompileMode{classparser.Precision, classparser.Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			sourceRoot := filepath.Join(root, "source-"+string(mode))
			metrics := gateMetrics{}

			for internalName, classBytes := range classes {
				resultHashes := make(map[string]bool)
				sourceHashes := make(map[string]bool)
				var source string

				for run := 0; run < gateRuns; run++ {
					result, err := classparser.DecompileWithOptions(classBytes, classparser.DecompileOptions{
						Mode: mode,
						Resolve: func(name string) ([]byte, bool) {
							data, ok := classes[name]
							return data, ok
						},
						TargetSourceVersion: 8,
					})
					if err != nil {
						t.Fatalf("fixture %q class %q mode %s decompile run %d/%d failed: status=%q diagnostics=%+v error=%v", fixture.File, internalName, mode, run+1, gateRuns, result.Status, result.Diagnostics, err)
					}
					assertHealthyDecompile(t, fixture.File, internalName, mode, run, result, fixture.AllowedDiagnostics[string(mode)][strings.ReplaceAll(internalName, "/", ".")])
					encoded, err := json.Marshal(result)
					if err != nil {
						t.Fatalf("fixture %q class %q mode %s result cannot be hashed: %v", fixture.File, internalName, mode, err)
					}
					resultHashes[hashBytes(encoded)] = true
					sourceHashes[hashBytes([]byte(result.Source))] = true
					if run == 0 {
						source = result.Source
					}
				}
				if len(resultHashes) != 1 || len(sourceHashes) != 1 {
					t.Fatalf("fixture %q class %q mode %s is nondeterministic across %d decompilations: source hashes=%v result hashes=%v", fixture.File, internalName, mode, gateRuns, sortedSet(sourceHashes), sortedSet(resultHashes))
				}
				metrics = addGateMetrics(metrics, sourceMetrics(source))
				writeGateFile(t, filepath.Join(sourceRoot, internalName+".java"), []byte(source))
			}
			assertGateMetrics(t, fixture.File, mode, metrics, fixture.Quality[string(mode)])

			sources := gateFiles(t, sourceRoot, ".java")

			parse := runGateProcess(t, root, java, append([]string{"-cp", helperRoot, "ParseOnly"}, sources...)...)
			assertGateProcess(t, fixture.File+" "+string(mode)+" ParseOnly", parse, 0, "PARSED_NOT_TYPECHECKED\n", "")
			rebuiltRoot := filepath.Join(root, "rebuilt-"+string(mode))
			if err := os.MkdirAll(rebuiltRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			compileArgs := []string{"-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", "8", "-classpath", rebuiltRoot, "-d", rebuiltRoot}
			compileArgs = append(compileArgs, sources...)
			compile := runGateProcess(t, root, javac, compileArgs...)
			assertGateProcess(t, fixture.File+" "+string(mode)+" whole-tree javac", compile, 0, "", "")
			rebuiltClasses := readClassTree(t, rebuiltRoot)
			if got := dottedClassNames(rebuiltClasses); !reflect.DeepEqual(got, fixture.Classes) {
				t.Fatalf("fixture %q mode %s rebuilt identities changed: got %q, want exactly %q; compiler output=%q", fixture.File, mode, got, fixture.Classes, sortedKeys(rebuiltClasses))
			}
			for internalName, data := range rebuiltClasses {
				if _, err := classparser.Parse(data); err != nil {
					t.Fatalf("fixture %q mode %s rebuilt class %q does not parse: %v", fixture.File, mode, internalName, err)
				}
			}
			verifyGateClasses(t, java, helperRoot, rebuiltRoot, fixture.Classes, fixture.File+" "+string(mode)+" rebuilt")
			repacked := filepath.Join(root, "repacked-"+string(mode)+".jar")
			writeRebuiltJar(t, repacked, rebuiltClasses)
			repackedEntries := readGateJar(t, fixture.File+" "+string(mode)+" rebuilt JAR", mustReadFile(t, repacked))
			if got := dottedClassNames(classEntries(repackedEntries)); !reflect.DeepEqual(got, fixture.Classes) {
				t.Fatalf("fixture %q mode %s repacked identities changed: got %q, want exactly %q; entries=%q", fixture.File, mode, got, fixture.Classes, sortedKeys(repackedEntries))
			}
			rebuiltRuns := runGatePrograms(t, java, javac, root, rebuiltRoot, fixture)
			if !reflect.DeepEqual(rebuiltRuns, translatedRuns) {
				t.Fatalf("fixture %q mode %s behavior differs between translator and rebuilt trees:\ntranslated=%+v\nrebuilt=%+v", fixture.File, mode, translatedRuns, rebuiltRuns)
			}

		})
	}
}

func assertHealthyDecompile(t *testing.T, fixture, class string, mode classparser.DecompileMode, run int, result classparser.DecompileResult, allowedDiagnostics []string) {
	t.Helper()
	allowed := make(map[string]int, len(allowedDiagnostics))
	for _, code := range allowedDiagnostics {
		allowed[code]++
	}
	var undeclared []classparser.DecompileDiagnostic
	for _, diagnostic := range result.Diagnostics {
		if allowed[diagnostic.Code] == 0 {
			undeclared = append(undeclared, diagnostic)
			continue
		}
		allowed[diagnostic.Code]--
	}
	var missing []string
	for code, count := range allowed {
		for ; count > 0; count-- {
			missing = append(missing, code)
		}
	}
	sort.Strings(missing)
	if result.Status != "complete" || len(result.StubMethods) != 0 || strings.TrimSpace(result.Source) == "" || len(undeclared) != 0 || len(missing) != 0 || len(result.RulesApplied) != 0 {
		t.Fatalf("fixture %q class %q mode %s decompile run %d is degraded or changed its diagnostic contract: status=%q stubs=%q diagnostics=%+v allowed=%q undeclared=%+v missing=%q rules=%+v source_bytes=%d", fixture, class, mode, run+1, result.Status, result.StubMethods, result.Diagnostics, allowedDiagnostics, undeclared, missing, result.RulesApplied, len(result.Source))
	}
	for _, member := range result.Members {
		if member.State != "preserved" && member.State != "regenerated" {
			t.Fatalf("fixture %q class %q mode %s decompile run %d has incomplete member %+v", fixture, class, mode, run+1, member)
		}
	}
	lower := strings.ToLower(result.Source)
	for _, marker := range []string{"yak-decompiler:", "decompile dump failed", "decompile parse failed", "undecompilable method body", "unsupported bootstrap"} {
		if strings.Contains(lower, marker) {
			t.Fatalf("fixture %q class %q mode %s decompile run %d source contains forbidden degradation marker %q:\n%s", fixture, class, mode, run+1, marker, result.Source)
		}
	}
}

func readGateManifest(t *testing.T) gateManifest {
	t.Helper()
	path := filepath.Join("testdata", "accept", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read strict gate manifest %q: %v", path, err)
	}
	var manifest gateManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode strict gate manifest %q: %v", path, err)
	}
	if manifest.Schema != 1 || len(manifest.Fixtures) == 0 {
		t.Fatalf("strict gate manifest has unsupported schema or empty corpus: schema=%d fixtures=%d", manifest.Schema, len(manifest.Fixtures))
	}
	return manifest
}

func assertGateFixtureInventory(t *testing.T, manifest gateManifest) {
	t.Helper()
	registered := make(map[string]bool)
	for _, fixture := range manifest.Fixtures {
		if fixture.File == "" || filepath.Base(fixture.File) != fixture.File {
			t.Fatalf("strict gate manifest fixture path must be one basename, got %q", fixture.File)
		}
		if registered[fixture.File] {
			t.Fatalf("strict gate manifest registers fixture %q more than once", fixture.File)
		}
		registered[fixture.File] = true
		info, err := os.Stat(filepath.Join("testdata", "accept", fixture.File))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("strict gate manifest fixture %q is missing or not a regular file: %v", fixture.File, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join("testdata", "accept"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]bool{"README.md": true, "SHA256SUMS": true, "manifest.json": true}
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() != "drivers" {
				t.Fatalf("unregistered directory in accepted corpus: %q", entry.Name())
			}
			continue
		}
		if !metadata[entry.Name()] && !registered[entry.Name()] {
			t.Fatalf("unregistered fixture in accepted corpus: %q; add it to manifest.json or remove it", entry.Name())
		}
	}
}

func inputInventory(t *testing.T, name string, data []byte) []string {
	t.Helper()
	if strings.HasSuffix(name, ".dex") {
		return []string{"<raw-dex>"}
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("fixture %q cannot be inventoried as APK: %v", name, err)
	}
	out := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		out = append(out, file.Name)
	}
	return out
}

func readGateJar(t *testing.T, fixture string, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("fixture %q translated output is not a JAR: %v", fixture, err)
	}
	entries := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		if _, exists := entries[file.Name]; exists {
			t.Fatalf("fixture %q translated JAR has duplicate entry %q", fixture, file.Name)
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatalf("fixture %q open translated JAR entry %q: %v", fixture, file.Name, err)
		}
		contents, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("fixture %q read translated JAR entry %q: read=%v close=%v", fixture, file.Name, readErr, closeErr)
		}
		entries[file.Name] = contents
	}
	return entries
}

func compileGateHelpers(t *testing.T, javac, root string) string {
	t.Helper()
	helperRoot := filepath.Join(root, "helpers")
	if err := os.MkdirAll(helperRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	parseOnly, err := filepath.Abs(filepath.Join("testdata", "helpers", "ParseOnly.java"))
	if err != nil {
		t.Fatal(err)
	}
	verifyOnly, err := filepath.Abs(filepath.Join("testdata", "helpers", "VerifyOnly.java"))
	if err != nil {
		t.Fatal(err)
	}
	result := runGateProcess(t, root, javac, "-proc:none", "-encoding", "UTF-8", "-d", helperRoot, parseOnly, verifyOnly)
	assertGateProcess(t, "compile ParseOnly/VerifyOnly helpers", result, 0, "", "")
	return helperRoot
}

func verifyGateClasses(t *testing.T, java, helperRoot, classRoot string, classes []string, label string) {
	t.Helper()
	args := []string{"-Xverify:all", "-cp", helperRoot + string(os.PathListSeparator) + classRoot, "VerifyOnly"}
	args = append(args, classes...)
	result := runGateProcess(t, classRoot, java, args...)
	assertGateProcess(t, label+" VerifyOnly", result, 0, "VERIFIED_NOT_EXECUTED\n", "")
}

func runGatePrograms(t *testing.T, java, javac, root, applicationRoot string, fixture gateFixture) []gateProcessResult {
	t.Helper()
	driverRoot := filepath.Join(root, "driver-"+filepath.Base(applicationRoot))
	if err := os.MkdirAll(driverRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	driverPath, err := filepath.Abs(filepath.Join("testdata", "accept", fixture.Driver))
	if err != nil {
		t.Fatal(err)
	}
	compile := runGateProcess(t, root, javac, "-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", "8", "-classpath", applicationRoot, "-d", driverRoot, driverPath)
	assertGateProcess(t, fixture.File+" driver compile against "+applicationRoot, compile, 0, "", "")
	classpath := driverRoot + string(os.PathListSeparator) + applicationRoot
	results := make([]gateProcessResult, 0, len(fixture.Runs))
	for _, run := range fixture.Runs {
		result := runGateProcess(t, root, java, "-Xverify:all", "-cp", classpath, run.Class)
		assertGateProcess(t, fixture.File+" run "+run.Class+" against "+applicationRoot, result, run.Exit, run.Stdout, run.Stderr)
		result.Duration = 0
		results = append(results, result)
	}
	return results
}

func writeRebuiltJar(t *testing.T, path string, classes map[string][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range sortedKeys(classes) {
		entry, err := writer.Create(name + ".class")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(classes[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func classEntries(entries map[string][]byte) map[string][]byte {
	classes := make(map[string][]byte)
	for name, data := range entries {
		if strings.HasSuffix(name, ".class") {
			classes[strings.TrimSuffix(name, ".class")] = data
		}
	}
	return classes
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requiredTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("strict DEX roundtrip gate requires %s; PATH lookup failed: %v", name, err)
	}
	return path
}

func runGateProcess(t *testing.T, cwd, executable string, args ...string) gateProcessResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gateProcessLimit)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = cwd
	command.Stdin = nil
	var stdout, stderr cappedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	err := command.Run()
	duration := time.Since(started)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("process timed out after %s: cwd=%q command=%q stdout=%q stderr=%q", gateProcessLimit, cwd, append([]string{executable}, args...), stdout.String(), stderr.String())
	}
	if errors.Is(err, errGateOutputLimit) || stdout.full || stderr.full {
		t.Fatalf("process exceeded %d-byte output cap: cwd=%q command=%q stdout=%q stderr=%q", gateProcessOutput, cwd, append([]string{executable}, args...), stdout.String(), stderr.String())
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			t.Fatalf("process could not start: cwd=%q command=%q error=%v stdout=%q stderr=%q", cwd, append([]string{executable}, args...), err, stdout.String(), stderr.String())
		}
	}
	return gateProcessResult{ExitCode: exit, Stdout: stdout.String(), Stderr: stderr.String(), Duration: duration}
}

type cappedBuffer struct {
	data bytes.Buffer
	full bool
}

var errGateOutputLimit = errors.New("strict gate process output limit exceeded")

func (b *cappedBuffer) Write(data []byte) (int, error) {
	remaining := gateProcessOutput - b.data.Len()
	if remaining <= 0 {
		b.full = true
		return 0, errGateOutputLimit
	}
	if len(data) > remaining {
		_, _ = b.data.Write(data[:remaining])
		b.full = true
		return remaining, errGateOutputLimit
	}
	return b.data.Write(data)
}

func (b *cappedBuffer) String() string { return b.data.String() }

func assertGateProcess(t *testing.T, label string, got gateProcessResult, exit int, stdout, stderr string) {
	t.Helper()
	if got.ExitCode != exit || got.Stdout != stdout || got.Stderr != stderr {
		t.Fatalf("%s result mismatch after %s: exit=%d want=%d stdout=%q want=%q stderr=%q want=%q", label, got.Duration, got.ExitCode, exit, got.Stdout, stdout, got.Stderr, stderr)
	}
}

func writeClassTree(t *testing.T, root string, classes map[string][]byte) {
	t.Helper()
	for internalName, data := range classes {
		writeGateFile(t, filepath.Join(root, filepath.FromSlash(internalName)+".class"), data)
	}
}

func writeGateFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readClassTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	for _, path := range gateFiles(t, root, ".class") {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		internalName := strings.TrimSuffix(filepath.ToSlash(relative), ".class")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out[internalName] = data
	}
	return out
}

func gateFiles(t *testing.T, root, suffix string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func dottedClassNames(classes map[string][]byte) []string {
	out := make([]string, 0, len(classes))
	for internalName := range classes {
		out = append(out, strings.ReplaceAll(internalName, "/", "."))
	}
	sort.Strings(out)
	return out
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortedKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedSet(values map[string]bool) []string { return sortedKeys(values) }

var (
	gateLabelPattern        = regexp.MustCompile(`(?m)^\s*[A-Za-z_$][A-Za-z0-9_$]*\s*:`)
	gateWhileTruePattern    = regexp.MustCompile(`\bwhile\s*\(\s*true\s*\)`)
	gateSyntheticVarPattern = regexp.MustCompile(`\bvar[0-9]+\b`)
	gateUnsupportedPattern  = regexp.MustCompile(`\bthrow\s+new\s+(?:java\.lang\.)?UnsupportedOperationException\b`)
)

func sourceMetrics(source string) gateMetrics {
	lines := 0
	if source != "" {
		lines = strings.Count(source, "\n")
		if !strings.HasSuffix(source, "\n") {
			lines++
		}
	}
	metrics := gateMetrics{
		Lines:             lines,
		Labels:            len(gateLabelPattern.FindAllStringIndex(source, -1)),
		WhileTrue:         len(gateWhileTruePattern.FindAllStringIndex(source, -1)),
		SyntheticVars:     len(gateSyntheticVarPattern.FindAllStringIndex(source, -1)),
		UnsupportedThrows: len(gateUnsupportedPattern.FindAllStringIndex(source, -1)),
	}
	inToken := false
	for _, r := range source {
		word := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
		if word && !inToken {
			metrics.Tokens++
		}
		inToken = word
	}
	return metrics
}

func addGateMetrics(left, right gateMetrics) gateMetrics {
	return gateMetrics{
		Lines: left.Lines + right.Lines, Tokens: left.Tokens + right.Tokens,
		Labels: left.Labels + right.Labels, WhileTrue: left.WhileTrue + right.WhileTrue,
		SyntheticVars:     left.SyntheticVars + right.SyntheticVars,
		UnsupportedThrows: left.UnsupportedThrows + right.UnsupportedThrows,
	}
}

func assertGateMetrics(t *testing.T, fixture string, mode classparser.DecompileMode, got gateMetrics, quality gateQuality) {
	t.Helper()
	if quality.Baseline == (gateMetrics{}) || quality.Maximum == (gateMetrics{}) {
		t.Fatalf("fixture %q mode %s has no source-quality baseline/maximum; observed %s", fixture, mode, formatGateMetrics(got))
	}
	if got != quality.Baseline {
		t.Fatalf("fixture %q mode %s source-quality baseline changed: got %s, manifest pins %s", fixture, mode, formatGateMetrics(got), formatGateMetrics(quality.Baseline))
	}
	maximum := quality.Maximum
	if got.Lines > maximum.Lines || got.Tokens > maximum.Tokens || got.Labels > maximum.Labels || got.WhileTrue > maximum.WhileTrue || got.SyntheticVars > maximum.SyntheticVars || got.UnsupportedThrows > maximum.UnsupportedThrows {
		t.Fatalf("fixture %q mode %s source quality regressed: got %s, manifest maximum %s", fixture, mode, formatGateMetrics(got), formatGateMetrics(maximum))
	}
}

func formatGateMetrics(metrics gateMetrics) string {
	return fmt.Sprintf("lines=%d tokens=%d labels=%d while_true=%d synthetic_vars=%d unsupported_throws=%d", metrics.Lines, metrics.Tokens, metrics.Labels, metrics.WhileTrue, metrics.SyntheticVars, metrics.UnsupportedThrows)
}
