// Package requiredci holds the small committed corpus exercised by required CI.
package requiredci_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/internal/jimagefixture"
	"github.com/yaklang/javajive/jimage"
)

type corpusManifest struct {
	Schema   int             `json:"schema"`
	Fixtures []corpusFixture `json:"fixtures"`
}

type corpusFixture struct {
	File               string                               `json:"file"`
	SHA256             string                               `json:"sha256"`
	Size               int                                  `json:"size"`
	JarSHA256          string                               `json:"jar_sha256"`
	JarInventory       []string                             `json:"jar_inventory"`
	InputInventory     []string                             `json:"input_inventory"`
	Report             map[string]int                       `json:"report"`
	Classes            []string                             `json:"classes"`
	AllowedDiagnostics map[string]map[string][]string       `json:"allowed_diagnostics"`
	Driver             string                               `json:"driver"`
	DriverClass        string                               `json:"driver_class"`
	Runs               []map[string]any                     `json:"runs"`
	Quality            map[string]map[string]map[string]int `json:"quality"`
}

func TestFixedDEXCorpusStructure(t *testing.T) {
	root := filepath.Join("..", "..", "dex2jar", "testdata", "accept")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest corpusManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("manifest has trailing JSON: %v", err)
	}
	if manifest.Schema != 1 || len(manifest.Fixtures) != 2 {
		t.Fatalf("unexpected fixed corpus manifest: schema=%d fixtures=%d", manifest.Schema, len(manifest.Fixtures))
	}
	seen := map[string]bool{}
	for _, fixture := range manifest.Fixtures {
		if seen[fixture.File] || filepath.Base(fixture.File) != fixture.File {
			t.Fatalf("unsafe or duplicate fixture name %q", fixture.File)
		}
		seen[fixture.File] = true
		raw, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) != fixture.Size || digest(raw) != fixture.SHA256 {
			t.Fatalf("fixture %s does not match its committed size/hash", fixture.File)
		}
		if got := inputInventory(t, fixture.File, raw); !reflect.DeepEqual(got, fixture.InputInventory) {
			t.Fatalf("fixture %s input inventory = %v, want %v", fixture.File, got, fixture.InputInventory)
		}
		first, report, err := dex2jar.DexToJar(raw, dex2jar.Options{})
		if err != nil {
			t.Fatalf("fixture %s conversion: %v", fixture.File, err)
		}
		second, secondReport, err := dex2jar.DexToJar(raw, dex2jar.Options{})
		if err != nil || report != secondReport || !bytes.Equal(first, second) {
			t.Fatalf("fixture %s conversion is not deterministic: %v", fixture.File, err)
		}
		if digest(first) != fixture.JarSHA256 {
			t.Fatalf("fixture %s JAR hash = %s, want %s", fixture.File, digest(first), fixture.JarSHA256)
		}
		if got := jarInventory(t, first); !reflect.DeepEqual(got, fixture.JarInventory) {
			t.Fatalf("fixture %s JAR inventory = %v, want %v", fixture.File, got, fixture.JarInventory)
		}
	}
}

func TestSyntheticJImageStructure(t *testing.T) {
	classBytes, err := classfilebuilder.NewClass(
		"java/lang/Object", "", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER,
	).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	imageBytes, err := jimagefixture.Build([]jimagefixture.Entry{
		{Path: "/java.base/java/lang/Object.class", Data: classBytes},
		{Path: "/java.base/conf/security/java.security", Data: []byte("fixed=true\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := jimage.OpenBytes(imageBytes)
	if err != nil {
		t.Fatal(err)
	}
	entries := reader.Entries()
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	var output bytes.Buffer
	if err := reader.WriteZIP(&output); err != nil {
		t.Fatal(err)
	}
	if got := jarInventory(t, output.Bytes()); !reflect.DeepEqual(got, []string{"java.base/conf/security/java.security", "java/lang/Object.class"}) {
		t.Fatalf("JImage ZIP inventory = %v", got)
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func inputInventory(t *testing.T, name string, data []byte) []string {
	t.Helper()
	if strings.HasSuffix(name, ".dex") {
		return []string{"<raw-dex>"}
	}
	return zipInventory(t, data, false)
}

func jarInventory(t *testing.T, data []byte) []string {
	t.Helper()
	return zipInventory(t, data, true)
}

func zipInventory(t *testing.T, data []byte, parseClasses bool) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	result := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		result = append(result, file.Name)
		if !parseClasses || !strings.HasSuffix(file.Name, ".class") {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		classBytes, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read %s: %v / %v", file.Name, readErr, closeErr)
		}
		if _, err := classparser.Parse(classBytes); err != nil {
			t.Fatalf("parse %s: %v", file.Name, err)
		}
	}
	sort.Strings(result)
	return result
}
