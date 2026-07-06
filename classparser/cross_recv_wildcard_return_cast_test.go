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
	outer, err := os.ReadFile("testdata/regression/CrossRecvWildcardSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	// Fix ON (default): the `(Class<A>)` cast is present on the getKind() return.
	os.Unsetenv("JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF")
	on, err := DecompileWithResolver(outer, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Class<A>) (this.holder.getKind())") {
		t.Errorf("fix ON: expected a `(Class<A>)` cast on the getKind() return, got:\n%s", on)
	}

	// Fix OFF: the cast disappears (the uncompilable bare return), proving it is load-bearing.
	t.Setenv("JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF", "1")
	off, err := DecompileWithResolver(outer, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Class<A>) (") {
		t.Errorf("fix OFF: expected NO `(Class<A>)` cast (kill-switch load-bearing), got:\n%s", off)
	}
}
