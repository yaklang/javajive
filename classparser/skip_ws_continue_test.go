package javaclassparser

// 承重测试: okhttp Util.skipLeading/TrailingAsciiWhitespace 的空白 case 落入
// default return, 循环 continue 不可达。kill-switch: JDEC_SKIP_WS_CONTINUE_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestSkipAsciiWhitespaceContinueIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Util.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_SKIP_WS_CONTINUE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "var3++;\n\t\t\t\t\tcontinue;\n\t\t\t\tdefault:") {
		t.Errorf("fix ON: expected whitespace case to continue before default, got:\n%s", on)
	}

	t.Setenv("JDEC_SKIP_WS_CONTINUE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "var3++;\n\t\t\t\t\tcontinue;\n\t\t\t\tdefault:") {
		t.Errorf("fix OFF: reconstruct survived the kill-switch, got:\n%s", off)
	}
	if !strings.Contains(off, "var3++;\n\t\t\t\tdefault:") {
		t.Errorf("fix OFF: expected fall-through into default, got:\n%s", off)
	}
}
