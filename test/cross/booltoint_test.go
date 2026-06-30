package cross

import "testing"

// TestBoolToIntCoerceIsLoadBearing pins guava DoubleMath (log2(double, RoundingMode)): an int local is
// assigned a non-short-circuit boolean connective `(exponent < 0) & !isPowerOfTwo(x)` originally written
// `increment = (...) ? 1 : 0`. The compiler that built guava left the boolean (already 0/1) on the stack
// with NO branch, so the decompiler recovers `int increment = c1 & c2` that javac rejects ("boolean
// cannot be converted to int"). CoerceIntAssignRHS re-inserts the elided `? 1 : 0`; disabling it via
// JDEC_BOOL_TO_INT_COERCE_OFF must reproduce the defect.
//
// Note: this defect can only be PINNED against a pre-built jar -- a modern javac (9+) always materialises
// `bool ? 1 : 0` with a branch (iand;ifeq;iconst;goto;iconst), so the no-branch `iand;istore` shape that
// triggers the bug is not authorable from Java source with the local toolchain.
func TestBoolToIntCoerceIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}
	const prefix = "com/google/common/math/DoubleMath"

	on := groupRecompileErrorsSwitch(t, jarPath, prefix, "JDEC_BOOL_TO_INT_COERCE_OFF", false) // fix ON
	off := groupRecompileErrorsSwitch(t, jarPath, prefix, "JDEC_BOOL_TO_INT_COERCE_OFF", true) // fix OFF
	t.Logf("DoubleMath group recompile errors: ON=%d OFF=%d", on, off)

	if off <= on {
		t.Errorf("bool->int coerce fix is NOT load-bearing: ON=%d OFF=%d (OFF must reproduce more errors)",
			on, off)
	}
}
