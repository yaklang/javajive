package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestCatchMergeWrappingRethrowIsLoadBearing pins the mergeNestedSameTypeCatches fix for a
// wrapping rethrow (`throw new RuntimeException(t)` instead of bare `throw t`). When the
// bytecode emits two catch(Throwable) handlers (the primary catch with a wrapping rethrow
// and the cleanup/finally handler), the merge must PRESERVE the wrapping throw (whose method
// declares `throws Throwable`, covering wildcard-capture checked exceptions) and insert the
// cleanup code BEFORE it. Without the fix, the merge drops the wrapping throw and replaces
// it with a bare `throw t`, causing "unreported exception CAP#1" when the caught exception
// is a wildcard-capture type. Kill-switch: JDEC_NO_CATCH_MERGE. Real hit: commons-lang3
// LockingVisitors.LockVisitor.lockAcceptUnlock.
func TestCatchMergeWrappingRethrowIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CatchMergeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_NO_CATCH_MERGE")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	// The merge should produce a single catch with the cleanup + wrapping throw.
	catchCount := strings.Count(on, "}catch(Throwable")
	if catchCount != 1 {
		t.Errorf("fix ON: expected 1 catch clause (merged), got %d:\n%s", catchCount, on)
	}
	// The cleanup code (unlock) should be inside the catch, before the throw.
	if !strings.Contains(on, "var1.unlock()") {
		t.Errorf("fix ON: expected unlock() in merged catch, got:\n%s", on)
	}
	// The wrapping throw should be preserved (not replaced with bare `throw var2`).
	if !strings.Contains(on, "throw new RuntimeException(var2)") {
		t.Errorf("fix ON: expected wrapping RuntimeException throw, got:\n%s", on)
	}

	os.Setenv("JDEC_NO_CATCH_MERGE", "1")
	defer os.Unsetenv("JDEC_NO_CATCH_MERGE")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	// Without the merge, there should be 2 catch clauses (unmerged).
	catchCountOff := strings.Count(off, "}catch(Throwable")
	if catchCountOff != 2 {
		t.Errorf("fix OFF: expected 2 catch clauses (unmerged), got %d:\n%s", catchCountOff, off)
	}
}
