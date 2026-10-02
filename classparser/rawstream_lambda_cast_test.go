package javaclassparser

// Original bootstrap evidence fixes both stream SAM entries to RawStreamItem.
// Materialized Function and Predicate targets must preserve those CHECKCASTs,
// including the method reference whose implementation accepts Object. A bare
// raw Objects::nonNull target would accept foreign payloads before later effects.

import (
	"os"
	"strings"
	"testing"
)

// TestRawStreamLambdaCastIsLoadBearing pins the raw-JDK-Stream-receiver lambda cast fix. The seed's local
// is declared RAW `List var2 = var1.getItems();` (the cross-class getItems() descriptor return is the
// erased raw List), so `var2.stream()` is a RAW Stream. With the fix ON the `.map(...)` LAMBDA is cast to
// its recovered parameterized functional type (Function<RawStreamItem, Object>), so it binds against the
// raw Stream's erased SAM; but a METHOD reference (`Objects::nonNull`) is left UNCAST because a method
// reference binds naturally to the raw SAM, and a parameterized-FI cast on it can defeat javac poly
// inference at SAMs with nested wildcards (Stream.flatMap's `Function<? super T, ? extends Stream<? extends
// R>>`, or Collectors.collect) -- see TestMethodRefFIcastIsLoadBearing. With the kill-switch OFF the bare
// `.map((l0) -> ...)` reappears, which javac rejects ("incompatible parameter types in lambda expression").
// Real hit: fastjson2 JSONPathSegment$CycleNameSegment$MapRecursive.
func TestRawStreamLambdaCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RawStreamLambdaSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedSAMInstantiation(t, data, "(Ljava/lang/Object;)Z => (Lrawstream/RawStreamItem;)Z", "(Ljava/lang/Object;)Ljava/lang/Object; => (Lrawstream/RawStreamItem;)Ljava/lang/Object;")
	// The original Predicate's instantiated SAM is RawStreamItem, including its
	// entry CHECKCAST. A raw Objects::nonNull target would remove that check.
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		carrier := reviewedFunctionalCarrier(t, source, "Function<RawStreamItem, Object>", `(`)
		if !strings.Contains(compact, ".map((Function)("+carrier+"))") || !strings.Contains(compact, "(Predicate<RawStreamItem>)(Objects::nonNull)") || !strings.Contains(compact, ".val()") {
			t.Fatalf("switch=%q: stream operations must retain both instantiated SAM entry types: %s", setting, source)
		}
	}
}
