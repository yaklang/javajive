package javajive_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jj "github.com/yaklang/javajive"
)

const helloStringStream = "aced000574000568656c6c6f"

func testdataClass(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("classparser", "testdata", "invisible_anno.class"))
	if err != nil {
		t.Fatalf("read testdata class: %v", err)
	}
	return data
}

func TestDecompileExport(t *testing.T) {
	src, err := jj.Decompile(testdataClass(t))
	if err != nil {
		t.Fatalf("Decompile: %v", err)
	}
	if !strings.Contains(src, "class InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}
}

func TestParseClassExport(t *testing.T) {
	obj, err := jj.ParseClass(testdataClass(t))
	if err != nil {
		t.Fatalf("ParseClass: %v", err)
	}
	if got := obj.GetClassName(); got != "InvisibleAnnoSeed" {
		t.Fatalf("class name = %q", got)
	}
	if got := obj.GetSupperClassName(); got != "java/lang/Object" {
		t.Fatalf("super name = %q", got)
	}
}

func TestSerializationRoundTripExport(t *testing.T) {
	objs, err := jj.ParseSerializedHex(helloStringStream)
	if err != nil {
		t.Fatalf("ParseSerializedHex: %v", err)
	}
	if got := jj.MarshalSerializedHex(objs...); got != helloStringStream {
		t.Fatalf("marshal round-trip = %s want %s", got, helloStringStream)
	}

	jsonBytes, err := jj.SerializedToJSON(objs...)
	if err != nil {
		t.Fatalf("SerializedToJSON: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "TC_STRING") {
		t.Fatalf("json missing TC_STRING: %s", jsonBytes)
	}

	restored, err := jj.SerializedFromJSON(jsonBytes)
	if err != nil {
		t.Fatalf("SerializedFromJSON: %v", err)
	}
	if got := jj.MarshalSerializedHex(restored...); got != helloStringStream {
		t.Fatalf("json round-trip = %s want %s", got, helloStringStream)
	}
}

func TestDecompileFileExport(t *testing.T) {
	src, err := jj.DecompileFile(filepath.Join("classparser", "testdata", "invisible_anno.class"))
	if err != nil {
		t.Fatalf("DecompileFile: %v", err)
	}
	if !strings.Contains(src, "InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}
}

func TestDecompileArchiveWithAnalysisBudgetExport(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "seed.jar")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entry, err := zw.Create("InvisibleAnnoSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(testdataClass(t)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jarPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "src")
	if err := jj.DecompileArchiveWithAnalysisBudget(jarPath, outDir, 2_000_000); err != nil {
		t.Fatalf("DecompileArchiveWithAnalysisBudget: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(outDir, "InvisibleAnnoSeed.java"))
	if err != nil {
		t.Fatalf("read decompiled source: %v", err)
	}
	if !strings.Contains(string(src), "class InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}

	if err := jj.DecompileArchiveWithAnalysisBudget(jarPath, outDir, -1); err == nil {
		t.Fatal("negative analysis budget accepted")
	}
}
