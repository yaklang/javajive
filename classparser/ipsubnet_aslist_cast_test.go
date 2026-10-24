package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Keep the historical inventory name, but require the original bytecode's
// Object[] check. The former source-name repair narrowed that check to the
// input's component type. Compilation did not prove this transformation safe:
// TestAdversarialArrayProducerResultCannotBorrowInputArrayType supplies an
// opaque <T>T producer and unchanged binary caller that refute it. This saved
// single class checks source-repair independence; the authored fixture checks
// compilation, complete declaration metadata, binding and runtime behavior.
func TestIpSubnetFilterAsListCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/IpSubnetFilter.class")
	if err != nil {
		t.Fatal(err)
	}
	original := arrayProducerCheckedInvocations(t, data)
	checked := 0
	for _, row := range strings.Split(original, "\n") {
		if strings.HasPrefix(row, "<init>(") && strings.Contains(row, ".asList([Ljava/lang/Object;)Ljava/util/List;") {
			if !strings.Contains(row, "checkcast:[Ljava/lang/Object;") || strings.Contains(row, "checkcast:[Lio/netty/handler/ipfilter/IpSubnetFilterRule;") {
				t.Fatal("saved original does not supply the expected broad array check", row)
			}
			checked++
		}
	}
	if checked != 2 {
		t.Fatalf("expected both original varargs constructor checks, got %d", checked)
	}
	for _, off := range []string{"", "1"} {
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(bytes.Clone(data), DecompileOptions{Mode: mode, TargetSourceVersion: 8, EnvSnapshot: map[string]string{"JDEC_NETTY_REMAINING_OFF": off}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(result.Source, "Arrays.asList(") || !strings.Contains(result.Source, "(Object[])") || strings.Contains(result.Source, "(IpSubnetFilterRule[])") {
				t.Fatalf("%s/remaining-off=%q must retain the original array view without component narrowing:\n%s", mode, off, result.Source)
			}
		}
	}
}
