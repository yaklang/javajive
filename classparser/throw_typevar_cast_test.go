package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"regexp"
	"strings"
	"testing"
)

// Original Signature/ATHROW binds the source formal to the same Throwable payload.
// The generic typed renderer owns both settings; the independent oracle checks
// checked exception/Error identity, null behavior and once-only producer effects.
func TestThrowTypeVarCastIsLoadBearing(t *testing.T) {
	const path = "testdata/regression/ThrowTypeVarCastSeed.class"
	const descriptor = "(Ljava/lang/Throwable;)Ljava/lang/Object;"
	raw, code, _ := reviewedFixtureMethod(t, path, "typeErasure", descriptor)
	assertReviewedTypeVarMethod(t, raw, "typeErasure", descriptor, "<R:Ljava/lang/Object;T:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)TR;^TT;")
	assertReviewedOpcode(t, code, 0, core.OP_ALOAD_0)
	assertReviewedOpcode(t, code, 1, core.OP_ATHROW)
	assertReviewedTypeVarInvoke(t, path, "wrapAndThrow", descriptor, 1, core.OP_INVOKESTATIC, "ThrowTypeVarCastSeed", "typeErasure", descriptor)
	assertReviewedSources(t, raw, "JDEC_FIX_THROW_TYPEVAR_CAST_OFF", func(source string) {
		body := reviewedControlBody(t, source, `private static <R, T extends Throwable> R typeErasure\(Throwable`)
		payload := requireReviewedPattern(t, body, `typeErasure\(Throwable\s+(\w+)\)`)[1]
		requireReviewedPattern(t, body, `throws T\s*\{\s*throw\s+\(T\)\s*\(*`+regexp.QuoteMeta(payload)+`\)*;\s*\}`)
		if strings.Count(body, "throw ") != 1 {
			t.Fatal("original ATHROW payload duplicated")
		}
	})
}
