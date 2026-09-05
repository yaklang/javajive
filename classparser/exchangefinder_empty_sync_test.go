package javaclassparser

// 承重测试: okhttp ExchangeFinder 的 synchronized 体被 CFG 掏空, findConnection /
// hasRouteToTry 缺 return。kill-switch: JDEC_EXCHANGEFINDER_SYNC_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestExchangeFinderEmptySyncIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ExchangeFinder.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_EXCHANGEFINDER_SYNC_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "return var7;") {
		t.Errorf("fix ON: expected findHealthyConnection/findConnection to return var7, got:\n%s", on)
	}
	if !strings.Contains(on, "this.routeSelector.hasNext()") {
		t.Errorf("fix ON: expected hasRouteToTry routeSelector.hasNext, got:\n%s", on)
	}
	if strings.Contains(on, "synchronized(var9){\n\n\t\t}") {
		t.Errorf("fix ON: empty synchronized findConnection body still present, got:\n%s", on)
	}

	t.Setenv("JDEC_EXCHANGEFINDER_SYNC_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "synchronized(var9){\n\n\t\t}") {
		t.Errorf("fix OFF: expected the empty synchronized findConnection body, got:\n%s", off)
	}
	if strings.Contains(off, "this.routeSelector.hasNext()") {
		t.Errorf("fix OFF: reconstruct survived the kill-switch, got:\n%s", off)
	}
}
