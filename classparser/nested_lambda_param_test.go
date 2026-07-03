package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestNestedLambdaParamScopeIsLoadBearing pins the nested-lambda parameter scoping fix
// (JDEC_LAMBDA_PARAM_SCOPE_OFF). javac forbids a lambda parameter from shadowing an enclosing lambda
// parameter that is in scope. Because a nested lambda's arrow is materialised eagerly while the outer
// lambda's bytecode is still being parsed, the flat `l0,l1,...` scheme names BOTH the outer and inner
// parameter `l0`, and the recompile fails with "variable l0 is already defined" (spring-core
// MergedAnnotationPredicates.typeIn, DataBufferUtils.readAsynchronousFileChannel). With the fix ON the
// nested lambda's parameters are namespaced by depth (`l2_0`); with the kill-switch OFF the flat name
// reappears and the same identifier is declared twice inside one method.
func TestNestedLambdaParamScopeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NestedLambdaParamSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the nested lambda parameter is depth-namespaced.
	os.Unsetenv("JDEC_LAMBDA_PARAM_SCOPE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "l2_0") {
		t.Errorf("fix ON: expected depth-namespaced nested lambda param `l2_0`, got:\n%s", on)
	}
	// In the `nested` method the outer param `l0` and the inner param must be DISTINCT and both present.
	if !strings.Contains(on, "l0.equals(l2_0)") {
		t.Errorf("fix ON: expected inner body to reference both outer `l0` and inner `l2_0`, got:\n%s", on)
	}

	// Fix OFF: the inner lambda falls back to the flat `l0`, colliding with the outer `l0`.
	t.Setenv("JDEC_LAMBDA_PARAM_SCOPE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "l2_0") {
		t.Errorf("fix OFF: expected NO depth-namespaced `l2_0` (kill-switch not load-bearing), got:\n%s", off)
	}
	// The `nested` method now declares two `l0` lambda parameters in the same scope (the defect).
	nestedDoubleL0 := regexp.MustCompile(`\(l0\) -> \{[\s\S]*\(l0\) -> \{`)
	if !nestedDoubleL0.MatchString(off) {
		t.Errorf("fix OFF: expected the flat `l0` to reappear on both nested lambdas, got:\n%s", off)
	}
}
