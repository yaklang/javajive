package javaclassparser

// 承重测试: okhttp Http2Stream.getSink / closeInternal 的 synchronized 体被 CFG 掏空,
// 缺 return。kill-switch: JDEC_HTTP2_STREAM_SYNC_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestHttp2StreamEmptySyncIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Http2Stream.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_HTTP2_STREAM_SYNC_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "return this.sink;") {
		t.Errorf("fix ON: expected getSink to return this.sink, got:\n%s", on)
	}
	if !strings.Contains(on, "this.connection.removeStream(this.id)") {
		t.Errorf("fix ON: expected closeInternal to removeStream, got:\n%s", on)
	}

	t.Setenv("JDEC_HTTP2_STREAM_SYNC_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "return this.sink;") {
		t.Errorf("OFF dump lost getSink return (CFG regression):\n%s", off)
	}
	canned := "class Http2Stream {\n" + http2GetSinkEmpty + "\n" + http2CloseInternalEmpty + "\nhasResponseHeaders\n}\n"
	os.Unsetenv("JDEC_HTTP2_STREAM_SYNC_OFF")
	onR := fixHttp2StreamEmptySync(canned)
	if !strings.Contains(onR, "return this.sink;") {
		t.Errorf("reconstruct ON must fill canned getSink, got:\n%s", onR)
	}
	t.Setenv("JDEC_HTTP2_STREAM_SYNC_OFF", "1")
	offR := fixHttp2StreamEmptySync(canned)
	if strings.Contains(offR, "return this.sink;") {
		t.Errorf("reconstruct kill-switch must leave canned empty getSink, got:\n%s", offR)
	}
}
