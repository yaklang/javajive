package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestNullAdoptedSubtypeReassignIsLoadBearing pins the null-adopted subtype reassignment merge
// (JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF) together with the java.io stream hierarchy added to
// hierarchy.go. A null-initialized slot adopts InputStream (`in = pick(..)`) and one arm then stores a
// SUBTYPE allocation (`in = new GZIPInputStream(in)`) into the same slot. With the fix ON the subtype
// store reuses the SAME `var4` (declared InputStream), so the post-merge `var4.read()` is assigned on
// every path. With the kill-switch the null-adopt-once guard mints a fresh `GZIPInputStream var5` for the
// wrapping arm and the post-merge read (`var5.read()`) is left unassigned on the non-gzip path — the
// definite-assignment bug this merge repairs (jsoup HttpConnection$Response.execute).
func TestNullAdoptedSubtypeReassignIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NullAdoptedSubtypeReassignSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "var4 = new GZIPInputStream(var4);") {
		t.Errorf("fix ON: expected the subtype store to reuse var4, got:\n%s", on)
	}

	t.Setenv("JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "GZIPInputStream var5;") {
		t.Errorf("fix OFF: expected a fresh split `GZIPInputStream var5` (kill-switch not load-bearing), got:\n%s", off)
	}
}
