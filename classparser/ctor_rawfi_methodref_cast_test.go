package javaclassparser

// A raw BiConsumer constructor parameter cannot supply the instantiated
// (Throwable, StackTraceElement[]) SAM of the original unbound method reference.
// Bootstrap-derived functional views remain required even with the superseded
// constructor spelling workaround disabled.

import (
	"os"
	"strings"
	"testing"
)

func TestCtorRawFIMethodRefCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MethodRefRawFICtorSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedSAMInstantiation(t, data, "(Ljava/lang/Object;Ljava/lang/Object;)V => (Ljava/lang/Throwable;[Ljava/lang/StackTraceElement;)V")
	// Bootstrap-derived functional binding supersedes this spelling workaround.
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_CTOR_RAWFI_METHODREF_CAST_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		if !strings.Contains(compact, `newMethodRefRawFICtorSeed$Sink("trace",`) || !strings.Contains(compact, "(BiConsumer<Throwable,StackTraceElement[]>)(Throwable::setStackTrace)") {
			t.Fatalf("switch=%q: original raw constructor needs bootstrap-exact functional view: %s", setting, source)
		}
	}
}
