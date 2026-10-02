package javaclassparser

// The original field is Seed<Comparable>, while all() returns Seed<C>.
// Their invariant declaration conflict requires an unchecked same-erasure raw
// bridge. The former spelling-only switch is retired; both states must preserve
// the binding contract. Trusted JVM identity/effect probes live in
// TestAdversarialReviewedGenericReturnBindingsRoundTrip.

import (
	"os"
	"strings"
	"testing"
)

func TestSameErasureFieldReturnBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SameErasureRetBridgeSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericMethod(t, data, "all", "()LSameErasureRetBridgeSeed;", "<C::Ljava/lang/Comparable;>()LSameErasureRetBridgeSeed<TC;>;")
	assertReviewedGenericField(t, data, "ALL", "LSameErasureRetBridgeSeed;", "LSameErasureRetBridgeSeed<Ljava/lang/Comparable;>;")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(compactReviewedGenericSource(source), "return(SameErasureRetBridgeSeed<C>)(SameErasureRetBridgeSeed)(ALL)") {
			t.Fatalf("switch=%q: invariant field/method declaration conflict requires a raw bridge: %s", setting, source)
		}
	}
}
