package cross

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// TestScratchDump 反编译指定 jar 里的若干 class 到持久目录, 供人工排错。
//
//	用法: SCRATCH_JAR=guava SCRATCH_CLASSES=com/google/common/collect/Maps,... SCRATCH_DIR=/tmp/jdec-scratch \
//	       go test -run TestScratchDump ./test/cross/
func TestScratchDump(t *testing.T) {
	jarName := os.Getenv("SCRATCH_JAR")
	classes := os.Getenv("SCRATCH_CLASSES")
	if jarName == "" || classes == "" {
		t.Skip("set SCRATCH_JAR and SCRATCH_CLASSES")
	}
	dir := os.Getenv("SCRATCH_DIR")
	if dir == "" {
		dir = "/tmp/jdec-scratch"
	}
	spec, ok := jarSpecs[jarName]
	if !ok {
		t.Fatalf("unknown jar %q", jarName)
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skipf("jar %s not found", spec.relPath)
	}
	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal: %v", err)
	}
	defer jfs.Close()
	os.MkdirAll(dir, 0o755)
	for _, c := range strings.Split(classes, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		entry := c
		if !strings.HasSuffix(entry, ".class") {
			entry += ".class"
		}
		raw, err := jfs.ReadFile(entry)
		if err != nil {
			t.Errorf("read %s: %v", entry, err)
			continue
		}
		out := filepath.Join(dir, strings.ReplaceAll(strings.TrimSuffix(c, ".class"), "/", ".")+".java")
		if err := os.WriteFile(out, raw, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Logf("wrote %s (%d bytes)", out, len(raw))
	}
}
