package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLambdaCaptureDescriptorTypesOnlyItsFreshOriginalOperand(t *testing.T) {
	for _, kind := range []string{"fresh reference", "fresh array", "same erasure generic", "missing witness", "wrong ordinal", "declared", "inlined", "different seed", "changed uid", "source declaration", "primitive"} {
		t.Run(kind, func(t *testing.T) {
			impl := &values.JavaClassMember{Name: "p.Owner", Member: "lambda$body", Description: "(Ljava/lang/ClassLoader;)Ljava/lang/Object;", RefKind: RefInvokeStatic}
			seed := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, seed.Type().Copy())
			if kind != "missing witness" && kind != "source declaration" {
				index := 0
				if kind == "wrong ordinal" {
					index = 1
				}
				snapshot.MarkOriginalDynamicOperand(10, index, seed)
			}
			plain := kind == "fresh reference" || kind == "fresh array" || kind == "same erasure generic" || kind == "primitive"
			switch kind {
			case "fresh array":
				impl.Description = "([Ljava/lang/String;)Ljava/lang/Object;"
			case "same erasure generic":
				impl.Description = "(Ljava/util/List;)Ljava/lang/Object;"
				snapshot.ResetVarType(types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "declared":
				snapshot.MarkOriginalDynamicOperandDeclaration(10, seed)
			case "inlined":
				snapshot.StackVar = seed
				snapshot.MarkOriginalDynamicOperandInline(10, seed)
			case "different seed":
				snapshot.Val = values.NewJavaRef(utils.NewRootVariableId(), nil, seed.Type())
			case "changed uid":
				snapshot.VarUid += "foreign"
			case "source declaration":
				snapshot = seed
			case "primitive":
				impl.Description = "(I)Ljava/lang/Object;"
			}
			beforeType := snapshot.Type()
			beforeSeed := snapshot.Val
			input := []values.JavaValue{snapshot}
			out := lambdaCaptureDescriptorViews(impl, input)
			if len(out) != 1 || input[0] != snapshot || snapshot.Val != beforeSeed {
				t.Fatal("changed original operand or its source seed")
			}
			if plain {
				if out[0] != snapshot {
					t.Fatal("owned snapshot acquired a separate descriptor-view identity")
				}
				want := "Ljava/lang/ClassLoader;"
				if kind == "fresh array" {
					want = "[Ljava/lang/String;"
				}
				if kind == "same erasure generic" {
					want = "Ljava/util/List;"
				}
				if kind == "primitive" {
					want = "Ljava/lang/Object;"
				}
				if actual, known := values.SourceTypeErasure(snapshot.Type(), &class_context.ClassContext{}); !known || actual != want {
					t.Fatalf("descriptor=%q known=%v expected=%q", actual, known, want)
				}
				if kind == "fresh reference" || kind == "fresh array" {
					if pc, index, known := snapshot.OriginalPendingDynamicOperand(seed); !known || pc != 10 || index != 0 {
						t.Fatal("descriptor typing replaced the private original operand witness")
					}
				}
				if kind == "same erasure generic" && snapshot.Type() != beforeType {
					t.Fatal("lost stronger source generic declaration")
				}
			} else {
				cast, ok := out[0].(*values.CastExpression)
				if !ok || !cast.Binding || cast.Value != snapshot || snapshot.Type() != beforeType {
					t.Fatal("unowned/completed/changed operand borrowed fresh snapshot typing")
				}
			}
			if actual, known := values.SourceTypeErasure(seed.Type(), &class_context.ClassContext{}); !known || actual != "Ljava/lang/Object;" {
				t.Fatal("changed the enclosing source declaration type")
			}
		})
	}
}
