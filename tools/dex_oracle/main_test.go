package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedManifest(t *testing.T) {
	manifest, hashValue, err := loadManifest("")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Oracles) != 5 {
		t.Fatalf("got %d oracles, want 5", len(manifest.Oracles))
	}
	if !hex64.MatchString(hashValue) {
		t.Fatalf("invalid manifest hash %q", hashValue)
	}
	want := []string{"official-2.0", "official-2.1", "official-2.3", "official-2.4", "fork-2.4.38"}
	for i, id := range want {
		if manifest.Oracles[i].ID != id {
			t.Fatalf("oracle[%d] id = %q, want %q", i, manifest.Oracles[i].ID, id)
		}
	}
}

func TestResultSchemaHasRequiredTopLevelFields(t *testing.T) {
	data, err := os.ReadFile("result.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"schema_version", "manifest_sha256", "oracles", "inputs", "results"} {
		if !contains(schema.Required, required) {
			t.Errorf("schema does not require %q", required)
		}
	}
}

func TestManifestGuards(t *testing.T) {
	valid := fakeOracle()
	tests := []struct {
		name   string
		mutate func(*Oracle)
	}{
		{"duplicate id", func(o *Oracle) {}},
		{"archive traversal", func(o *Oracle) { o.ArchiveName = "../fake.zip" }},
		{"launcher traversal", func(o *Oracle) { o.Launcher = "../run" }},
		{"bad hash", func(o *Oracle) { o.SHA256 = strings.Repeat("0", 63) }},
		{"insecure URL", func(o *Oracle) { o.URL = "http://example.com/fake.zip" }},
		{"URL name mismatch", func(o *Oracle) { o.URL = "https://example.com/other.zip" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := valid
			tt.mutate(&o)
			manifest := Manifest{SchemaVersion: 1, Oracles: []Oracle{o}}
			if tt.name == "duplicate id" {
				manifest.Oracles = append(manifest.Oracles, o)
			}
			if err := validateManifest(manifest); err == nil {
				t.Fatal("validateManifest unexpectedly succeeded")
			}
		})
	}
}

func TestCleanJavaEnv(t *testing.T) {
	got := cleanJavaEnv([]string{
		"PATH=/usr/bin",
		"JAVA_TOOL_OPTIONS=-javaagent:unknown.jar",
		"JDK_JAVA_OPTIONS=-Xmx9g",
		"_JAVA_OPTIONS=-Duser.home=/unknown",
		"KEEP=value=with=equals",
	})
	want := []string{"PATH=/usr/bin", "KEEP=value=with=equals"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("cleanJavaEnv() = %q, want %q", got, want)
	}
}

func TestSelectOraclesRejectsUnknown(t *testing.T) {
	manifest := Manifest{SchemaVersion: 1, Oracles: []Oracle{fakeOracle()}}
	if _, err := selectOracles(manifest, "missing"); err == nil {
		t.Fatal("selectOracles unexpectedly accepted unknown oracle")
	}
}

func TestVerifyFileHashAndSizeGuards(t *testing.T) {
	name := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(name, []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("archive"))
	if err := verifyFile(name, hex.EncodeToString(sum[:]), 7); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(name, strings.Repeat("0", 64), 7); err == nil {
		t.Fatal("verifyFile accepted wrong hash")
	}
	if err := verifyFile(name, hex.EncodeToString(sum[:]), 8); err == nil {
		t.Fatal("verifyFile accepted wrong size")
	}
}

func TestExtractRejectsTraversalAndMissingLauncher(t *testing.T) {
	oracle := fakeOracle()
	for _, tt := range []struct {
		name    string
		entries map[string][]byte
	}{
		{"traversal", map[string][]byte{"fake/../../escape": []byte("x"), "fake/run.sh": []byte("x")}},
		{"missing launcher", map[string][]byte{"fake/lib/fake.jar": []byte("x")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "fake.zip")
			writeZip(t, archive, tt.entries)
			if err := extractAtomic(archive, t.TempDir(), "dist", oracle); err == nil {
				t.Fatal("extractAtomic unexpectedly succeeded")
			}
		})
	}
}

func TestFakeDistributionProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake Java executable is a POSIX shell script")
	}
	classData := minimalClass(t)
	generatedJar := filepath.Join(t.TempDir(), "generated.jar")
	writeZip(t, generatedJar, map[string][]byte{
		"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\n"),
		"Fake.class":           classData,
	})

	archive := filepath.Join(t.TempDir(), "fake.zip")
	writeZip(t, archive, map[string][]byte{
		"fake/run.sh":       []byte("launcher discovery only\n"),
		"fake/lib/fake.jar": []byte("not executed by the fake java process\n"),
	})
	archiveData, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archiveData)
	oracle := fakeOracle()
	oracle.ArchiveSize = int64(len(archiveData))
	oracle.SHA256 = hex.EncodeToString(sum[:])

	cache := t.TempDir()
	archiveDir := filepath.Join(cache, "archives")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cachedArchive := filepath.Join(archiveDir, oracle.SHA256+"-"+oracle.ArchiveName)
	if err := os.WriteFile(cachedArchive, archiveData, 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := ensureOracle(context.Background(), cache, oracle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(record.Launcher, filepath.Join("fake", "run.sh")) {
		t.Fatalf("unexpected launcher %q", record.Launcher)
	}

	fakeJava := filepath.Join(t.TempDir(), "java")
	script := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then\n    shift\n    cp \"$FAKE_ORACLE_JAR\" \"$1\"\n    printf 'fake stdout\\n'\n    printf 'fake stderr\\n' >&2\n    exit 0\n  fi\n  shift\ndone\nexit 2\n"
	if err := os.WriteFile(fakeJava, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_ORACLE_JAR", generatedJar)
	inputFile := filepath.Join(t.TempDir(), "input.dex")
	if err := os.WriteFile(inputFile, []byte("dex\n035\x00fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	input, err := identifyInput(inputFile)
	if err != nil {
		t.Fatal(err)
	}
	result, err := probeOne(oracle, record.Distribution, input, fakeJava, 5*time.Second, defaultOutputCap)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.TimedOut || result.OutputLimited || !result.OutputJar.Present {
		t.Fatalf("unexpected probe result: %+v", result)
	}
	if len(result.OutputJar.Classes) != 1 {
		t.Fatalf("got %d classes, want 1", len(result.OutputJar.Classes))
	}
	class := result.OutputJar.Classes[0]
	if class.Name != "Fake" || len(class.Fields) != 1 || class.Fields[0].Name != "field" || class.Fields[0].Descriptor != "I" {
		t.Fatalf("unexpected class inventory: %+v", class)
	}
	if len(class.Methods) != 1 || class.Methods[0].Name != "method" || class.Methods[0].Descriptor != "()V" {
		t.Fatalf("unexpected method inventory: %+v", class.Methods)
	}
}

func fakeOracle() Oracle {
	return Oracle{
		ID: "fake-1", Family: "test", Version: "1", Repository: "https://github.com/example/example",
		ReleaseTag: "v1", SourceCommit: strings.Repeat("1", 40), URL: "https://example.com/fake.zip",
		ArchiveName: "fake.zip", ArchiveSize: 1, SHA256: strings.Repeat("2", 64), License: "Apache-2.0",
		DistributionRoot: "fake", Launcher: "run.sh",
	}
}

func writeZip(t *testing.T, name string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for entry, data := range entries {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func minimalClass(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	mustWrite := func(value any) {
		if err := binary.Write(&b, binary.BigEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	utf := func(value string) {
		mustWrite(uint8(1))
		mustWrite(uint16(len(value)))
		b.WriteString(value)
	}
	mustWrite(uint32(0xcafebabe))
	mustWrite(uint16(0))
	mustWrite(uint16(52))
	mustWrite(uint16(9))
	utf("Fake")
	mustWrite(uint8(7))
	mustWrite(uint16(1))
	utf("java/lang/Object")
	mustWrite(uint8(7))
	mustWrite(uint16(3))
	utf("field")
	utf("I")
	utf("method")
	utf("()V")
	mustWrite(uint16(0x0021))
	mustWrite(uint16(2))
	mustWrite(uint16(4))
	mustWrite(uint16(0))
	mustWrite(uint16(1))
	mustWrite(uint16(0x0001))
	mustWrite(uint16(5))
	mustWrite(uint16(6))
	mustWrite(uint16(0))
	mustWrite(uint16(1))
	mustWrite(uint16(0x0001))
	mustWrite(uint16(7))
	mustWrite(uint16(8))
	mustWrite(uint16(0))
	mustWrite(uint16(0))
	return b.Bytes()
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
