package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyPureDeclarationDoesNotInventEvaluationOrStoreEvidence(t *testing.T) {
	for _, variant := range []string{"primitive", "reference", "array type", "not a declaration", "first store", "initializer", "array store", "field", "parameter", "this", "nil identity", "custom ref", "stack ref", "missing ref"} {
		t.Run(variant, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			decl := statements.NewDeclareStatement(ref)
			switch variant {
			case "reference":
				ref.ResetVarType(types.NewJavaClass("example.Value"))
			case "array type":
				ref.ResetVarType(types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
			case "not a declaration":
				decl.IsDeclare = false
			case "first store":
				decl.IsFirst = true
			case "initializer":
				decl.JavaValue = numericConversionCall(15)
			case "array store":
				decl.ArrayMember = &values.JavaArrayMember{}
			case "field":
				decl.LeftValue = &values.RefMember{Object: ref, Member: "value", JavaType: ref.Type()}
			case "parameter":
				ref.IsParam = true
			case "this":
				ref.IsThis = true
			case "nil identity":
				ref.Id = nil
			case "custom ref":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
			case "stack ref":
				ref.StackVar = values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			case "missing ref":
				decl.LeftValue = nil
			}
			want := variant == "primitive" || variant == "reference" || variant == "array type"
			got := finallyCoveredAssignment(decl, func(int) bool { panic("pure declaration has no evaluation PC") })
			if got != want {
				t.Fatalf("declaration role=%v want=%v", got, want)
			}
		})
	}
}

func TestFinallyInertLocalCopiesNeedExactSourceTypesWithoutInventedPC(t *testing.T) {
	for _, variant := range []string{"primitive literal", "local copy", "reference copy", "array copy", "null reference", "null primitive", "boxing", "unboxing", "source target view", "source operand view", "call", "cast", "array store", "field store", "custom target", "stack target", "missing identity", "missing value"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			left := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			right := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			store := statements.NewAssignStatement(left, right, false)
			switch variant {
			case "primitive literal":
				store.JavaValue = values.NewJavaLiteral(7, integer)
			case "reference copy":
				left.ResetVarType(types.NewJavaClass("java.lang.Object"))
				right.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "array copy":
				left.ResetVarType(types.NewJavaArrayType(integer))
				right.ResetVarType(types.NewJavaArrayType(integer))
			case "null reference":
				left.ResetVarType(types.NewJavaClass("java.lang.Throwable"))
				store.JavaValue = values.JavaNull
			case "null primitive":
				store.JavaValue = values.JavaNull
			case "boxing":
				left.ResetVarType(types.NewJavaClass("java.lang.Integer"))
			case "unboxing":
				right.ResetVarType(types.NewJavaClass("java.lang.Integer"))
			case "source target view":
				left.WebDeclType = types.NewJavaClass("java.lang.Integer")
			case "source operand view":
				right.WebDeclType = types.NewJavaClass("java.lang.Integer")
			case "call":
				store.JavaValue = numericConversionCall(15)
			case "cast":
				store.JavaValue = &values.CastExpression{Value: right, TargetType: integer}
			case "array store":
				store.LeftValue = nil
				store.ArrayMember = &values.JavaArrayMember{}
			case "field store":
				store.LeftValue = &values.RefMember{Object: left, Member: "word", JavaType: integer}
			case "custom target":
				left.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return integer })
			case "stack target":
				left.StackVar = right
			case "missing identity":
				left.Id = nil
			case "missing value":
				store.JavaValue = nil
			}
			want := variant == "primitive literal" || variant == "local copy" || variant == "reference copy" || variant == "array copy" || variant == "null reference"
			if got := finallyCoveredAssignment(store, func(int) bool { return false }); got != want || store.HasOriginPC {
				t.Fatalf("covered=%v expected=%v invented PC=%v", got, want, store.HasOriginPC)
			}
		})
	}
}
