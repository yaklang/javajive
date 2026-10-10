package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Preserve the original seed as well as an executable oracle. Its F/f arm
// falls through to D/d when Float cannot represent the value. Returning null
// to make damaged output compile would hide precisely that missing edge.
func TestSwitchBreakMissingReturnSeedPreservesContinuation(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/SwitchBreakMissingReturnSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	source, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "return Double.valueOf(var0);") {
		t.Fatalf("lost real fallback return:\n%s", source)
	}
	t.Setenv("JDEC_FIX_SWITCH_BREAK_RETURN_OFF", "1")
	without, err := Decompile(raw)
	if err != nil || without != source {
		t.Fatalf("source still depends on synthesized returns: %v", err)
	}
}

func TestAdversarialSwitchFallthroughReturnsActualValueRoundTrip(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/regression/SwitchBreakMissingReturnSeed.java")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.TrimSpace(string(raw))
	source = source[:len(source)-1] + `
 public static void main(String[] args) {
   for(String input:new String[]{null,"12","2L","1.5F","0F","1e50F","1.5D","badQ"}) {
     try {
       Number value=parse(input);
       System.out.print(value==null ? "null;" : value.getClass().getSimpleName()+":"+value+";");
     } catch(NumberFormatException e) {System.out.print("invalid;");}
   }
 }
}`
	roundTripGenericFlow(t, "SwitchBreakMissingReturnSeed", source, Precision, Compatibility, "legacy")
}
