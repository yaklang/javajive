package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestThrowsTypeVarRecoveryIsLoadBearing pins the Signature-attribute throws-type-variable
// recovery. A generic method `<T extends Throwable> void accept(..., T) throws T` whose
// ExceptionsAttribute carries the erasure `java.lang.Throwable` must be rendered as `throws T`
// (from the Signature `^TT;`), not `throws Throwable`. Without this, a call site like
// `accept(obj::wait, ...)` where `obj::wait` throws `InterruptedException` fails with
// "unreported exception Throwable" because javac sees the method throwing `Throwable`
// instead of inferring `T = InterruptedException`. The fix reads the `^`-prefixed throws
// types from the method Signature attribute and overrides the ExceptionsAttribute-derived
// `throws` clause. Kill-switch: JDEC_THROWS_SIG_RECOVERY_OFF. Real hit: commons-lang3
// DurationUtils.accept -> ObjectUtils.wait.
func TestThrowsTypeVarRecoveryIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ThrowsTypeVarSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_THROWS_SIG_RECOVERY_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "throws T") {
		t.Errorf("fix ON: expected `throws T` in decompiled output, got:\n%s", on)
	}

	os.Setenv("JDEC_THROWS_SIG_RECOVERY_OFF", "1")
	defer os.Unsetenv("JDEC_THROWS_SIG_RECOVERY_OFF")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "throws Throwable") {
		t.Errorf("fix OFF: expected `throws Throwable` in decompiled output, got:\n%s", off)
	}
}
