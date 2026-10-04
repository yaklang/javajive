package javaclassparser

// The original Comparator SAM is instantiated as (String,String)I. Preserve
// that target on its materialized carrier before the field assignment. The
// still-active diagnostic toggle erases the carrier and remains a negative
// control, regardless of where the raw field bridge happens to be printed.

import (
	"os"
	"strings"
	"testing"
)

func TestComparatorMethodRefInstantiatedTypeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NaturalOrderSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedSAMInstantiation(t, data, "(Ljava/lang/Object;Ljava/lang/Object;)I => (Ljava/lang/String;Ljava/lang/String;)I")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		declared := "Comparator<String>"
		if setting != "" {
			declared = "Comparator"
		}
		carrier := reviewedFunctionalCarrier(t, source, declared, `String::compareTo;`)
		compact := compactReviewedGenericSource(source)
		if setting == "" && !strings.Contains(compact, "NATURAL_ORDER="+carrier+";") {
			t.Fatalf("typed comparator carrier must feed original field: %s", source)
		}
		if setting != "" && !strings.Contains(compact, "NATURAL_ORDER=(Comparator)("+carrier+");") {
			t.Fatalf("active negative control must expose erased comparator carrier: %s", source)
		}
	}
}
