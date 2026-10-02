package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Both normal definitions and subtype updates belong to one def-use web.
// Its spelling and the retired null-adoption kill-switch do not define correctness.
func TestNullAdoptedSubtypeReassignIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NullAdoptedSubtypeReassignSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`(\w+) = new GZIPInputStream\((\w+)\);`).FindStringSubmatch(source)
		if len(match) != 3 || match[1] != match[2] || !strings.Contains(source, match[1]+".read()") {
			t.Fatalf("subtype store disconnected from post-merge read:\n%s", source)
		}
	}
}
