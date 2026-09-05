package javaclassparser

// 承重测试: 非 void 方法以空 synchronized 体结尾时补 default return。
// 镜像 okhttp DiskLruCache$Editor.newSource / newSink。
// kill-switch: JDEC_EMPTY_SYNC_RETURN_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestEmptySyncMissingReturnIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/DiskLruCache$Editor.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_EMPTY_SYNC_RETURN_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "synchronized(var2){\n\n\t\t}\n\t\treturn null;") &&
		!strings.Contains(on, "synchronized(var2){\n\n\t\t}\n\t\treturn null;") {
		if !strings.Contains(on, "return null;") {
			t.Errorf("fix ON: expected default return after empty synchronized, got:\n%s", on)
		}
	}

	t.Setenv("JDEC_EMPTY_SYNC_RETURN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Count(off, "return null;") >= strings.Count(on, "return null;") &&
		strings.Contains(on, "return null;") && !strings.Contains(off, "public Source newSource") {
		t.Errorf("fix OFF: could not find newSource, got:\n%s", off)
	}
	// OFF: newSource/newSink still end at the empty synchronized with no return.
	if !strings.Contains(off, "public Source newSource(int var1) {\n\t\tDiskLruCache var2 = this.this$0;\n\t\tDiskLruCache var3 = var2;\n\t\tsynchronized(var2){\n\n\t\t}\n\t}") {
		t.Errorf("fix OFF: expected empty-sync newSource without a return, got:\n%s", off)
	}
}
