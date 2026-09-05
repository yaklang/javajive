package javaclassparser

// 承重测试: okhttp DiskLruCache$3.hasNext 的 else 分支空 synchronized 缺 return。
// kill-switch: JDEC_DISKLRU_ITER_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestDiskLruIteratorHasNextIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/DiskLruCache$3.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_DISKLRU_ITER_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.nextSnapshot = var2;") {
		t.Errorf("fix ON: expected hasNext to assign nextSnapshot, got:\n%s", on)
	}

	t.Setenv("JDEC_DISKLRU_ITER_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "this.nextSnapshot = var2;") {
		t.Errorf("fix OFF: reconstruct survived the kill-switch, got:\n%s", off)
	}
	if !strings.Contains(off, "synchronized(this.this$0){\n\n\t\t\t}") {
		t.Errorf("fix OFF: expected empty synchronized hasNext body, got:\n%s", off)
	}
}
