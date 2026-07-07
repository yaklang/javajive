package javaclassparser

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
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the map LAMBDA carries the parameterized functional-interface cast.
	os.Unsetenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Function<RawStreamItem, Object>)((l0) ->") {
		t.Errorf("fix ON: expected the map lambda cast to Function<RawStreamItem, Object>, got:\n%s", on)
	}
	// A method reference must NOT be cast: it binds naturally to the raw SAM, and the cast can break
	// poly inference at nested-wildcard SAMs.
	if strings.Contains(on, "(Predicate<RawStreamItem>)(Objects::nonNull)") {
		t.Errorf("fix ON: method reference must not be cast, got the unwanted cast:\n%s", on)
	}
	if !strings.Contains(on, ".filter(Objects::nonNull)") {
		t.Errorf("fix ON: expected the bare method reference `.filter(Objects::nonNull)`, got:\n%s", on)
	}

	// Fix OFF: the bare (uncastable) stream lambda reappears, proving the cast is load-bearing.
	t.Setenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Function<RawStreamItem, Object>)") {
		t.Errorf("fix OFF: expected the bare stream lambda, but the cast survived (kill-switch not load-bearing):\n%s", off)
	}
	if !strings.Contains(off, ".map((l0) ->") {
		t.Errorf("fix OFF: expected the bare `.map((l0) ->`, got:\n%s", off)
	}
}
