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
