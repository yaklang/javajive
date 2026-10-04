package javaclassparser

// Both doPrivileged overloads accept zero-input poly expressions. Materialize
// the original PrivilegedAction or give it an explicit target before overload
// selection. Either form preserves the same binding; inline-cast counts do not.

import (
	"os"
	"strings"
	"testing"
)

func TestDoPrivilegedLambdaCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/DoPrivilegedLambdaSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedSAMInstantiation(t, data, "()Ljava/lang/Object; => ()Ljava/security/ProtectionDomain;", "()Ljava/lang/Object; => ()Ljava/io/InputStream;")
	// A materialized FI local and an explicit FI cast both fix overload lookup.
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		carrier := reviewedFunctionalCarrier(t, source, "PrivilegedAction", `() ->`)
		if strings.Count(compact, "AccessController.doPrivileged(") != 2 || !strings.Contains(compact, "AccessController.doPrivileged("+carrier+")") || !strings.Contains(compact, "AccessController.doPrivileged((PrivilegedAction)") {
			t.Fatalf("switch=%q: both calls must retain their original FI overload binding: %s", setting, source)
		}
		if !strings.Contains(source, "getProtectionDomain") || !strings.Contains(source, "getResourceAsStream") {
			t.Fatalf("original producers changed: %s", source)
		}
	}
}
