package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestUnboundMethodRefObjectMethodCastIsLoadBearing pins the lambdaArgRawJDKReceiverCast
// fix for unbound instance method references that target Object-inherited methods (toString,
// hashCode, etc.) on a raw JDK generic receiver (Stream/Optional). When the method name IS on
// Object, javac tries to bind the reference to Object's version (0 args) instead of the unbound
// form (1 arg = receiver), causing "invalid method reference". The cast
// `(Function<Method, String>) Method::toString` re-targets the SAM. When the method name is NOT
// on Object (e.g. `MergedAnnotation::withNonMergedAttributes`), javac resolves correctly and
// the cast is skipped (it would break downstream type inference). Kill-switch:
// JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF. Real hit: commons-lang3 MethodUtils `map(Method::toString)`.
func TestUnboundMethodRefObjectMethodCastIsLoadBearing(t *testing.T) {
	// Build a minimal seed class that has a raw List → stream → map(Method::toString) pattern.
	// We use a pre-built seed for determinism.
	data, err := os.ReadFile("testdata/regression/UnboundMethodRefSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Function") || !strings.Contains(on, "::toString") {
		t.Errorf("fix ON: expected (Function<...>) Method::toString cast, got:\n%s", on)
	}

	os.Setenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF", "1")
	defer os.Unsetenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Function") {
		t.Errorf("fix OFF: expected bare Method::toString (no cast), got:\n%s", off)
	}
}
