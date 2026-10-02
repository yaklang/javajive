package javaclassparser

// Invoking through a raw generic receiver erases the formal FI declaration.
// A separately materialized Consumer<Elem> keeps the original instantiated SAM
// and captured body valid without an extra inline cast at the invocation.

import (
	"os"
	"strings"
	"testing"
)

func rawRecvLambdaDecompile(t *testing.T) string {
	t.Helper()
	seed, err := os.ReadFile("testdata/regression/RawRecvLambdaSeed.class")
	if err != nil {
		t.Fatalf("read RawRecvLambdaSeed seed: %v", err)
	}
	// Expose the sibling generic class RawRecvBox so SiblingClassSig can confirm it is generic.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}
	out, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	return out
}

func TestLambdaRawReceiverCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RawRecvLambdaSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedSAMInstantiation(t, data, "(Ljava/lang/Object;)V => (LElem;)V")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_LAMBDA_RAWRECV_CAST_OFF", setting)
		source := rawRecvLambdaDecompile(t)
		carrier := reviewedFunctionalCarrier(t, source, "Consumer<Elem>", `(`)
		compact := compactReviewedGenericSource(source)
		if !strings.Contains(compact, ".apply("+carrier+")") || !strings.Contains(compact, ".flag") || !strings.Contains(compact, ".add(") || !strings.Contains(compact, ".name") {
			t.Fatalf("switch=%q: materialized Consumer must preserve the lambda's entry type and captured body: %s", setting, source)
		}
	}
}
