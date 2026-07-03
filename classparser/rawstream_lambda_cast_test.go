package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestRawStreamLambdaCastIsLoadBearing pins the raw-JDK-Stream-receiver lambda cast fix. The seed's local
// is declared RAW `List var2 = var1.getItems();` (the cross-class getItems() descriptor return is the
// erased raw List), so `var2.stream()` is a RAW Stream. With the fix ON the `.filter(...)` /`.map(...)`
// lambdas are cast to their recovered parameterized functional types (Predicate<RawStreamItem> /
// Function<RawStreamItem, Object>), so they bind against the raw Stream's erased SAMs. With the
// kill-switch OFF the bare `.map((l0) -> ...)` reappears, which javac rejects
// ("incompatible parameter types in lambda expression"). Real hit: fastjson2
// JSONPathSegment$CycleNameSegment$MapRecursive.
func TestRawStreamLambdaCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RawStreamLambdaSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): both stream lambdas carry the parameterized functional-interface cast.
	os.Unsetenv("JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Function<RawStreamItem, Object>)((l0) ->") {
		t.Errorf("fix ON: expected the map lambda cast to Function<RawStreamItem, Object>, got:\n%s", on)
	}
	if !strings.Contains(on, "(Predicate<RawStreamItem>)(Objects::nonNull)") {
		t.Errorf("fix ON: expected the filter method-ref cast to Predicate<RawStreamItem>, got:\n%s", on)
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
