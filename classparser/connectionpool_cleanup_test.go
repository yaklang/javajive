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
	// CFG now keeps pruneAndGetAllocationCount inside the monitor (bytecode
	// monitorenter then invoke pruneAndGetAllocationCount). The leftover
	// reconstruct is still gated on the canned empty-sync shape.
	if !strings.Contains(off, "this.pruneAndGetAllocationCount(var8,var1)") {
		t.Errorf("OFF dump lost cleanup body (CFG regression):\n%s", off)
	}
	canned := "class RealConnectionPool {\n" + connectionPoolCleanupEmpty + "\nvoid pruneAndGetAllocationCount() {}\n}\n"
	os.Unsetenv("JDEC_CONNECTIONPOOL_CLEANUP_OFF")
	onR := fixConnectionPoolCleanup(canned)
	if !strings.Contains(onR, "this.pruneAndGetAllocationCount(var8,var1)") {
		t.Errorf("reconstruct ON must fill canned empty cleanup, got:\n%s", onR)
	}
	t.Setenv("JDEC_CONNECTIONPOOL_CLEANUP_OFF", "1")
	offR := fixConnectionPoolCleanup(canned)
	if strings.Contains(offR, "this.pruneAndGetAllocationCount(var8,var1)") {
		t.Errorf("reconstruct kill-switch must leave canned empty cleanup, got:\n%s", offR)
	}
}
