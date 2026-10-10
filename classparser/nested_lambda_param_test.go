package javaclassparser

import (
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
	raw := reviewedRemainingSAMRaw(t, "NestedLambdaParamSeed")
	assertReviewedTypeVarMethod(t, raw, "nested", "(Ljava/lang/String;)Ljava/util/function/Predicate;", "(Ljava/lang/String;)Ljava/util/function/Predicate<Ljava/lang/String;>;")
	assertReviewedSeedSAM(t, raw, "(Ljava/lang/Object;)Z", "(Ljava/lang/String;)Z")
	t.Setenv("JDEC_LAMBDA_PARAM_SCOPE_OFF", "")
	source, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	body := reviewedSourceMethod(t, source, `Predicate<String>\s+nested\(`)
	lambdas := regexp.MustCompile(`Predicate<String>\s+\w+\s*=\s*\((\w+)\)\s*->`).FindAllStringSubmatch(body, -1)
	if len(lambdas) != 2 || lambdas[0][1] == lambdas[1][1] {
		t.Fatal("nested lambda has colliding lexical parameter identities")
	}
	outer, inner := lambdas[0][1], lambdas[1][1]
	alias := requireReviewedPattern(t, body, `String\s+(\w+)\s*=\s*`+regexp.QuoteMeta(outer)+`;`)[1]
	captured := requireReviewedPattern(t, body, `final\s+String\s+(\w+)\s*=\s*`+regexp.QuoteMeta(alias)+`;`)[1]
	requireReviewedPattern(t, body, regexp.QuoteMeta(captured)+`\.equals\(`+regexp.QuoteMeta(inner)+`\)`)
	if !strings.Contains(body, inner+".startsWith(") {
		t.Fatal("inner SAM argument lost string binding")
	}
	t.Setenv("JDEC_LAMBDA_PARAM_SCOPE_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	bad := reviewedSourceMethod(t, off, `Predicate<String>\s+nested\(`)
	collisions := regexp.MustCompile(`Predicate<String>\s+\w+\s*=\s*\((\w+)\)\s*->`).FindAllStringSubmatch(bad, -1)
	if len(collisions) != 2 || collisions[0][1] != collisions[1][1] {
		t.Fatal("active OFF control lost original nested lexical shadowing counterexample")
	}
}
