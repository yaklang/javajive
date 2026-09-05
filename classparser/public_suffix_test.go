package javaclassparser

// 承重测试: okhttp PublicSuffixDatabase.findMatchingRule 空 sync 缺 return, 以及
// readTheListUninterruptibly 的 readTheList() 落到 IOException try 外。
// kill-switch: JDEC_PUBLIC_SUFFIX_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestPublicSuffixDatabaseIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/PublicSuffixDatabase.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_PUBLIC_SUFFIX_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.readTheList();\n\t\t\t\tif ((var1) != (0))") {
		t.Errorf("fix ON: expected readTheList inside the try, got:\n%s", on)
	}

	t.Setenv("JDEC_PUBLIC_SUFFIX_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "this.readTheList();\n\t\t\t\tif ((var1) != (0))") {
		t.Errorf("fix OFF: reconstruct survived the kill-switch, got:\n%s", off)
	}
	if !strings.Contains(off, "if(false)throw new IOException();") {
		t.Errorf("fix OFF: expected sentinel try body, got:\n%s", off)
	}
}
