package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestRawAssignLambdaCastIsLoadBearing pins the raw-functional-interface reassignment cast fix. The
// seed's local is declared RAW `Function var2 = this.builder;` (the field is a raw java.util.function
// .Function), then reassigned a method reference and an explicitly-typed lambda. With the fix ON each
// reassignment is cast to its recovered parameterized `Function<Collection, Collection>` (the exact form
// the javac-visible source uses), so both bind against the raw SAM. With the kill-switch OFF the bare
// forms reappear, which javac rejects as "invalid method reference" / "incompatible parameter types in
// lambda expression". Real hit: fastjson2 ObjectReaderImplList (builder =
// (Function<Collection, Collection>) Collections::unmodifiableCollection / ((Collection list) -> ...)).
func TestRawAssignLambdaCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RawAssignLambdaSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): both the method reference and the lambda carry the parameterized cast.
	os.Unsetenv("JDEC_LAMBDA_ASSIGN_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Function<Collection, Collection>)(Collections::unmodifiableCollection)") {
		t.Errorf("fix ON: expected the method reference to be cast to Function<Collection, Collection>, got:\n%s", on)
	}
	if !strings.Contains(on, "(Function<Collection, Collection>)((Collection l0) ->") {
		t.Errorf("fix ON: expected the lambda to be cast to Function<Collection, Collection>, got:\n%s", on)
	}

	// Fix OFF: the bare (uncastable) method reference / lambda reappear, proving the cast is load-bearing.
	t.Setenv("JDEC_LAMBDA_ASSIGN_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Function<Collection, Collection>)(Collections::unmodifiableCollection)") {
		t.Errorf("fix OFF: expected the bare method reference, but the cast survived (kill-switch not load-bearing):\n%s", off)
	}
	if !strings.Contains(off, "= Collections::unmodifiableCollection;") {
		t.Errorf("fix OFF: expected the bare `= Collections::unmodifiableCollection;`, got:\n%s", off)
	}
}
