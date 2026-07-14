package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestThrowableCatchMergeIsLoadBearing pins the Throwable-family catch-slot supertype-arm merge
// (reachingRefSlotThrowableArmMerge; kill-switch JDEC_REF_SLOT_THROWABLE_ARM_MERGE_OFF). A try/catch whose
// handlers write different exception types into one slot is a single logical `Throwable cause`; without
// the merge the arms split (an InterruptedException var + a Throwable var) and the post-catch
// `cause instanceof X` / `(X) cause` bind to the narrow var, which javac rejects ("InterruptedException
// cannot be converted to ..."). With the fix ON the slot is one `Throwable` variable. Mirrors spring core
// codec Decoder.decode / Encoder.encode.
func TestThrowableCatchMergeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ThrowableCatchMergeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// The kill-switch splits the catch slot into two DECLARED locals (`InterruptedException var2;` +
	// `Throwable var2_1;`); the fix collapses them to a single `Throwable var3;`. The `catch(...)` clause
	// parameter is unaffected either way, so assert on the leading local declaration instead.
	os.Unsetenv("JDEC_REF_SLOT_THROWABLE_ARM_MERGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "Throwable var3") {
		t.Errorf("fix ON: expected a single `Throwable var3` catch variable, got:\n%s", on)
	}
	if strings.Contains(on, "\t\tInterruptedException var") {
		t.Errorf("fix ON: expected NO split InterruptedException-typed local declaration, got:\n%s", on)
	}

	t.Setenv("JDEC_REF_SLOT_THROWABLE_ARM_MERGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "\t\tInterruptedException var") {
		t.Errorf("fix OFF: expected the split InterruptedException-typed local declaration (kill-switch not load-bearing), got:\n%s", off)
	}
}
