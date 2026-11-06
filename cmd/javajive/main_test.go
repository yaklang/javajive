package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdDexToJarHelp(t *testing.T) {
	if err := cmdDexToJar([]string{"-h"}); err != flag.ErrHelp {
		t.Fatalf("cmdDexToJar(-h) error = %v", err)
	}
}

func TestCmdDexToJarArguments(t *testing.T) {
	if err := cmdDexToJar(nil); err == nil || !strings.Contains(err.Error(), "requires -o") {
		t.Fatalf("cmdDexToJar(nil) error = %v", err)
	}
	if err := cmdDexToJar([]string{"input.dex"}); err == nil || !strings.Contains(err.Error(), "requires -o") {
		t.Fatalf("cmdDexToJar without output error = %v", err)
	}
}

func TestCmdDexToJarRejectsMagicWithoutOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "renamed.dex")
	output := filepath.Join(dir, "output.jar")
	if err := os.WriteFile(input, []byte("not dex"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdDexToJar([]string{"-o", output, input}); err == nil || !strings.Contains(err.Error(), "unsupported input magic") {
		t.Fatalf("cmdDexToJar error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output exists after failure: %v", err)
	}
}

func TestCmdDecompileAnalysisBudget(t *testing.T) {
	classPath, err := filepath.Abs(filepath.Join("..", "..", "classparser", "testdata", "invisible_anno.class"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "InvisibleAnnoSeed.java")
	if err := cmdDecompile([]string{"-analysis-budget", "2000000", "-o", out, classPath}); err != nil {
		t.Fatalf("cmdDecompile with budget: %v", err)
	}
	src, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "class InvisibleAnnoSeed") {
		t.Fatalf("unexpected source:\n%s", src)
	}

	if err := cmdDecompile([]string{"-analysis-budget", "-1", classPath}); err == nil ||
		!strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("negative budget error = %v", err)
	}

	// Directory inputs route every class through the same budgeted path.
	inDir := t.TempDir()
	raw, err := os.ReadFile(classPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inDir, "InvisibleAnnoSeed.class"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "src")
	if err := cmdDecompile([]string{"-analysis-budget", "2000000", "-o", outDir, inDir}); err != nil {
		t.Fatalf("cmdDecompile directory with budget: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "InvisibleAnnoSeed.java")); err != nil {
		t.Fatalf("directory output missing: %v", err)
	}
}
