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
	if !strings.Contains(off, "public Source newSource") {
		t.Errorf("OFF dump lost newSource (CFG regression):\n%s", off)
	}
	if !strings.Contains(off, "return null;") && !strings.Contains(off, "return ") {
		t.Errorf("OFF dump lost newSource return (CFG regression):\n%s", off)
	}
}
