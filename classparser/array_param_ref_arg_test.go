package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"strings"
	"testing"
)

// Null propagation may eliminate a dead local, but the call must still bind
// the original array overload. Its typed null is supplied by call binding even
// when the old local-argument workaround is disabled.
func TestArrayParamRefArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ArrayParamRefArgSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarMethod(t, data, "sizeOf", "([BIII)I", "")
	assertReviewedTypeVarInvoke(t, "testdata/regression/ArrayParamRefArgSeed.class", "size", "()I", 15, core.OP_INVOKEVIRTUAL, "ArrayParamRefArgSeed", "sizeOf", "([BIII)I")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedSourceMethod(t, source, `int\s+size\(\)`)
		if !strings.Contains(compactReviewedGenericSource(body), "this.sizeOf((byte[])(null),") {
			t.Fatalf("array-overload witness missing:\n%s", body)
		}
	}
}
