package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestLambdaImplicitParamsIsLoadBearing pins the implicit-lambda-parameter fix. The bytecode only
// preserves the ERASED impl-method descriptor (e.g. `Predicate` for a SAM `matches(Predicate<String>)`),
// so an EXPLICIT-typed arrow `(Predicate l0) -> ...` fails to bind against the parameterized SAM
// ("incompatible parameter types in lambda expression" -- spring-core ProfilesParser and friends, 21
// sites). Rendering the parameters WITHOUT types lets Java infer them from the target functional
// interface. With the kill-switch OFF the explicit erased type reappears.
func TestLambdaImplicitParamsIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/LambdaImplicitSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): implicit lambda parameters.
	os.Unsetenv("JDEC_LAMBDA_IMPLICIT_PARAMS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(l0) -> {") {
		t.Errorf("fix ON: expected implicit lambda param `(l0) -> {`, got:\n%s", on)
	}
	if strings.Contains(on, "(Predicate l0)") {
		t.Errorf("fix ON: expected NO explicit erased param type, but found `(Predicate l0)`:\n%s", on)
	}

	// Fix OFF: the explicit erased parameter type reappears, proving the fix is load-bearing.
	t.Setenv("JDEC_LAMBDA_IMPLICIT_PARAMS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "(Predicate l0)") {
		t.Errorf("fix OFF: expected explicit `(Predicate l0)` to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
