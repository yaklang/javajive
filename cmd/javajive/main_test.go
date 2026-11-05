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
