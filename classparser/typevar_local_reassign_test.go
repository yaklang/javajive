package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestTypeVarLocalReassignCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ChainedTransformer.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarMethod(t, data, "transform", "(Ljava/lang/Object;)Ljava/lang/Object;", "(TT;)TT;")
	assertReviewedTypeVarInvoke(t, "testdata/regression/ChainedTransformer.class", "transform", "(Ljava/lang/Object;)Ljava/lang/Object;", 26, core.OP_INVOKEINTERFACE, "org/apache/commons/collections4/Transformer", "transform", "(Ljava/lang/Object;)Ljava/lang/Object;")
	_, code, _ := reviewedFixtureMethod(t, "testdata/regression/ChainedTransformer.class", "transform", "(Ljava/lang/Object;)Ljava/lang/Object;")
	assertReviewedOpcode(t, code, 31, core.OP_ASTORE_1)
	assertReviewedOpcode(t, code, 39, core.OP_ARETURN)

	assertReviewedGenericField(t, data, "iTransformers", "[Lorg/apache/commons/collections4/Transformer;", "[Lorg/apache/commons/collections4/Transformer<-TT;+TT;>;")
	var on string
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_TYPEVAR_LOCAL_REASSIGN_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		match := regexp.MustCompile(`Ttransform\(T([A-Za-z_$][A-Za-z0-9_$]*)\)`).FindStringSubmatch(compact)
		if len(match) != 2 || !strings.Contains(compact, match[1]+"=(T)(") || !strings.Contains(compact, ".transform("+match[1]+"))") || !strings.Contains(compact, "return"+match[1]+";") {
			t.Fatal("erased transformer result must update the original T parameter and its returned identity")
		}
		if setting == "" {
			on = source
		} else if source != on {
			t.Fatal("retired local-reassign gate must not change declaration-proven binding")
		}
	}
}
