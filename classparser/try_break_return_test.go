package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A nonthrowing return may move past a loop without changing the protected
// region. Callable.call and its checkcast must remain inside the try; forcing
// the return itself into that region incorrectly constrains valid lowering.
func TestTryBreakReturnIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/TryBreakReturnSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedHandlerMultiplicity(t, raw, "JDEC_FIX_TRY_BREAK_RETURN_OFF")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_FIX_TRY_BREAK_RETURN_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		begin := strings.Index(source, "try{")
		end := strings.Index(source, "}catch(CancellationException")
		if begin < 0 || end <= begin || !strings.Contains(source[begin:end], ".call()") {
			t.Fatalf("switch=%q: throwing call moved outside its original handler scope:\n%s", setting, source)
		}
		if !strings.Contains(source, "continue;") || !strings.Contains(source, ".getCause()") || !strings.Contains(source, "return ") {
			t.Fatalf("switch=%q: retry, wrapped failure or successful return lost:\n%s", setting, source)
		}
	}
}
