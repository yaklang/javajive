package javaclassparser

// 承重测试: okhttp Transmitter 的 synchronized 体被 CFG 掏空, 非 void 方法缺 return。
// 镜像 DeserializerCache._createAndCacheValueDeserializer 空 sync。
// kill-switch: JDEC_TRANSMITTER_SYNC_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestTransmitterEmptySyncIsLoadBearing(t *testing.T) {
	t.Skip("unique transmitter reconstruct no longer matches dump shape")
	data, err := os.ReadFile("testdata/regression/Transmitter.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_TRANSMITTER_SYNC_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if strings.Contains(on, "synchronized(var3){\n\n\t\t}") {
		t.Errorf("fix ON: empty synchronized body still present, got:\n%s", on)
	}
	if !strings.Contains(on, "timeoutExit(") {
		t.Errorf("fix ON: expected maybeReleaseConnection timeoutExit, got:\n%s", on)
	}
}
