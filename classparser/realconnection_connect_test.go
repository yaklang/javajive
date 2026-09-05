package javaclassparser

// 承重测试: okhttp RealConnection.connect 的 try 体被掏成 `if(false) throw IOException;
// break;`, connectTunnel/connectSocket 落到 try 外, javac 报 unreported IOException。
// kill-switch: JDEC_REALCONNECTION_CONNECT_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestRealConnectionConnectIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RealConnection.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_REALCONNECTION_CONNECT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.establishProtocol(var10,var4,var6,var7);") {
		t.Errorf("fix ON: expected establishProtocol inside connect()'s try, got:\n%s", on)
	}
	if strings.Contains(on, "if(false)throw new IOException();\n\t\t\t\t\tbreak;") {
		t.Errorf("fix ON: sentinel connect() try body still present, got:\n%s", on)
	}

	t.Setenv("JDEC_REALCONNECTION_CONNECT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "if(false)throw new IOException();\n\t\t\t\t\tbreak;") {
		t.Errorf("fix OFF: expected the sentinel connect() try body, got:\n%s", off)
	}
	if strings.Contains(off, "this.establishProtocol(var10,var4,var6,var7);") {
		t.Errorf("fix OFF: reconstruct survived the kill-switch, got:\n%s", off)
	}
}
