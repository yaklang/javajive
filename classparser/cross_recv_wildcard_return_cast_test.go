package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestCrossRecvWildcardReturnCastIsLoadBearing pins crossRecvWildcardReturnCast. `CrossRecvWildcardSeed`
// declares `Class<A> getType()` whose body returns `this.holder.getKind()`, an instance call on a NON-`this`
// receiver (the field `holder`, of the jar-internal class `Holder`) whose recovered generic return is the
// WILDCARD parameterization `Class<? extends Annotation>` -- the SAME erasure (Class) as the declared
// `Class<A>`. javac captures the wildcard to CAP#1 and rejects `Class<CAP#1>` -> `Class<A>`, so the source
// carried an unchecked `(Class<A>)` cast the bytecode dropped. The fix recovers the callee return through
// the cross-class sibling resolver and re-inserts the cast; the kill-switch drops it, proving it
// load-bearing. Requires the resolver so the sibling `Holder` signature is visible. Real hit: spring-core
// TypeMappedAnnotation.getType() -> this.mapping.getAnnotationType().
func TestCrossRecvWildcardReturnCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CrossRecvWildcardSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericMethod(t, data, "getType", "()Ljava/lang/Class;", "()Ljava/lang/Class<TA;>;")
	resolver := func(name string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + name + ".class")
		return b, e == nil
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF", setting)
		source, err := DecompileWithResolver(data, resolver)
		if err != nil {
			t.Fatal(err)
		}
		hasView := strings.Contains(compactReviewedGenericSource(source), "return(Class<A>)(Class)(this.holder.getKind())")
		if hasView != (setting == "") {
			t.Fatalf("switch=%q: missing receiver-preserving wildcard return bridge: %s", setting, source)
		}
	}
}
