package javaclassparser

// 承重测试: okhttp RealConnectionPool.cleanup 的 synchronized 体被 CFG 掏空, long
// 方法缺 return。kill-switch: JDEC_CONNECTIONPOOL_CLEANUP_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestConnectionPoolCleanupIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RealConnectionPool.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_CONNECTIONPOOL_CLEANUP_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.pruneAndGetAllocationCount(var8,var1)") {
		t.Errorf("fix ON: expected cleanup to iterate pruneAndGetAllocationCount, got:\n%s", on)
	}
	if strings.Contains(on, "synchronized(this){\n\n\t\t}") {
		t.Errorf("fix ON: empty synchronized cleanup body still present, got:\n%s", on)
	}

	t.Setenv("JDEC_CONNECTIONPOOL_CLEANUP_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "synchronized(this){\n\n\t\t}") {
		t.Errorf("fix OFF: expected the empty synchronized cleanup body, got:\n%s", off)
	}
	if strings.Contains(off, "this.pruneAndGetAllocationCount(var8,var1)") {
		t.Errorf("fix OFF: reconstruct survived the kill-switch, got:\n%s", off)
	}
}
