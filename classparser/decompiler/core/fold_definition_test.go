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
