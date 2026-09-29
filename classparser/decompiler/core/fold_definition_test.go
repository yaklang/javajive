package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestFoldDefinitionPreservesAliasAndReplacement(t *testing.T) {
	typ := types.NewJavaClass("java.lang.Object")
	initial := values.NewJavaLiteral("initial", typ)
	source := values.NewJavaRef(utils.NewRootVariableId(), initial, typ)
	load := values.NewSlotValue(source, typ)
	copy := values.NewJavaRef(utils.NewRootVariableId(), load, typ)
	folded := foldDefinitionValue(copy)
	if folded != load {
		t.Fatalf("copy was chased through source definition: %T", folded)
	}
	// A later legitimate rewrite of the source must still reach the copy.
	replacement := values.NewJavaLiteral("replacement", typ)
	load.ResetValue(replacement)
	if values.UnpackSoltValue(folded) != replacement {
		t.Fatal("lost source replacement callback")
	}
	parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	if foldDefinitionValue(parameter) != parameter {
		t.Fatal("parameter lost its identity")
	}
}

func TestArrayFoldSelectsOnlySurvivingUse(t *testing.T) {
	array := values.NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.String")))
	array.HasEvaluationEndPC, array.EvaluationEndPC = true, 11
	removed := &VarFoldRule{CurrentOpcode: &OpCode{CurrentOffset: 5}}
	live := &VarFoldRule{CurrentOpcode: &OpCode{CurrentOffset: 13}}
	if got := soleArrayUseAfterInitializer(array, []*VarFoldRule{removed, live}); got != live {
		t.Fatal("selected removed element-store load instead of the live argument")
	}
	for _, pairs := range [][]*VarFoldRule{{removed}, {live, live}, {removed, nil}} {
		if soleArrayUseAfterInitializer(array, pairs) != nil {
			t.Fatal("accepted an ambiguous or absent surviving read")
		}
	}
	array.HasEvaluationEndPC = false
	if soleArrayUseAfterInitializer(array, []*VarFoldRule{live}) != nil {
		t.Fatal("accepted an incomplete array")
	}
}
