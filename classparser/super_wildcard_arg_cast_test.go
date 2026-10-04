package javaclassparser

// The original lower-bound consumer invokes apply(Object). An erased receiver
// view is valid only if it retains that tuple and adds no payload check; an E
// cast is not the unique valid source spelling. The old wildcard spelling gate
// is superseded, so both settings must retain the same proven invocation.

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Match the explicit original Object argument view, independent of local names.
var superWildcardArgCastRe = regexp.MustCompile(`apply\(\(Object\)\([A-Za-z_$][A-Za-z0-9_$]*\)\)`)

func TestSuperWildcardArgCastIsLoadBearing(t *testing.T) {
	names := []string{"SuperWildcardSink", "SuperWildcardSeed"}
	bytesByName := map[string][]byte{}
	for _, n := range names {
		data, err := os.ReadFile("testdata/regression/" + n + ".class")
		if err != nil {
			t.Fatalf("read seed %s: %v", n, err)
		}
		bytesByName[n] = data
	}
	// Resolver feeds every sibling seed's bytes by binary internal name (default package -> bare name),
	// so the cross-class walk can read SuperWildcardSink's generic signature (`apply(T)`).
	resolver := func(internalName string) ([]byte, bool) {
		data, ok := bytesByName[internalName]
		return data, ok
	}
	implBytes := bytesByName["SuperWildcardSeed"]

	// Signature and exact invocation evidence constrain the receiver/argument views.
	assertReviewedGenericField(t, implBytes, "sink", "LSuperWildcardSink;", "LSuperWildcardSink<-TE;>;")
	assertReviewedTypeVarMethod(t, implBytes, "check", "(Ljava/lang/Object;)Z", "")
	assertReviewedTypeVarMethod(t, bytesByName["SuperWildcardSink"], "apply", "(Ljava/lang/Object;)Z", "(TT;)Z")
	assertReviewedTypeVarInvoke(t, "testdata/regression/SuperWildcardSeed.class", "check", "(Ljava/lang/Object;)Z", 5, core.OP_INVOKEINTERFACE, "SuperWildcardSink", "apply", "(Ljava/lang/Object;)Z")

	t.Setenv("JDEC_GENERIC_SUPERWILDCARD_OFF", "")
	t.Setenv("JDEC_GENERIC_RESOLVE_OFF", "")
	on, err := DecompileWithResolver(implBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !superWildcardArgCastRe.MatchString(compactReviewedGenericSource(on)) || !strings.Contains(compactReviewedGenericSource(on), "((SuperWildcardSink)(this.sink)).apply(") {
		t.Errorf("expected original erased receiver and Object argument consumer view, got:\n%s", on)
	}

	t.Setenv("JDEC_GENERIC_SUPERWILDCARD_OFF", "1")
	off, err := DecompileWithResolver(implBytes, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if off != on {
		t.Fatal("retired wildcard spelling gate must not change the exact erased consumer binding")
	}
}
