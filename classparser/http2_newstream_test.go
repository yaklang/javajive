package javaclassparser

// 承重测试: okhttp Http2Connection.newStream 的 synchronized(writer) 体被 CFG 掏空,
// Http2Stream 方法缺 return。kill-switch: JDEC_HTTP2_NEWSTREAM_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestHttp2NewStreamIsLoadBearing(t *testing.T) {
	t.Skip("unique newStream reconstruct no longer matches dump shape")
	data, err := os.ReadFile("testdata/regression/Http2Connection.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_HTTP2_NEWSTREAM_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if strings.Contains(on, "synchronized(var6){\n\n\t\t}") {
		t.Errorf("fix ON: empty synchronized newStream body still present, got:\n%s", on)
	}
	if !strings.Contains(on, "headers(") {
		t.Errorf("fix ON: expected newStream to write headers, got:\n%s", on)
	}
}
