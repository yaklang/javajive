package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalDynamicNullViewKeepsOperandAndDeclarationRoles(t *testing.T) {
	for _, change := range []string{"original", "changed value", "changed type", "different seed", "changed uid", "inlined", "wrong index", "wrong factory", "primitive"} {
		t.Run(change, func(t *testing.T) {
			source := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			r := NewJavaRef(utils.NewRootVariableId(), source, source.Type().Copy())
			r.MarkOriginalDynamicOperand(19, 0, source)
			pc, index := 19, 0
			target := types.NewJavaClass("java.lang.ClassLoader")
			if change == "wrong index" {
				index = 1
			}
			if change == "wrong factory" {
				pc = 20
			}
			if change == "primitive" {
				target = types.NewJavaPrimer(types.JavaInteger)
			}
			marked := r.MarkOriginalNullDynamicOperandView(pc, index, source, target)
			if change == "wrong index" || change == "wrong factory" || change == "primitive" {
				if marked {
					t.Fatal("unowned projection")
				}
				return
			}
			if !marked {
				t.Fatal("missing owned view")
			}
			v, _, known := r.OriginalDynamicOperandSourceView(source)
			if !known || r.Val != source {
				t.Fatal("original seed replaced")
			}
			switch change {
			case "changed value":
				v.Data = "changed"
			case "changed type":
				v.JavaType = types.NewJavaClass("java.lang.String")
			case "different seed":
				r.Val = JavaNull
			case "changed uid":
				r.VarUid += "foreign"
			case "inlined":
				r.StackVar = source
				r.MarkOriginalDynamicOperandInline(19, source)
			}
			_, _, known = r.OriginalDynamicOperandSourceView(source)
			if known != (change == "original") {
				t.Fatal("modified source view borrowed identity")
			}
			if change != "original" {
				return
			}
			r.MarkOriginalDynamicOperandDeclaration(19, v)
			copy := *r
			if !copy.OriginalDynamicOperandDeclarationOf(r, v) || copy.OriginalDynamicOperandDeclarationOf(r, NewJavaLiteral("null", target)) {
				t.Fatal("declaration proof accepted arbitrary cast or lost its owned source view")
			}
			if _, _, ok := r.OriginalDynamicOperandWitness(source); !ok {
				t.Fatal("logical operand lost")
			}
			if r.MarkOriginalNullDynamicOperandView(19, 0, source, target) {
				t.Fatal("declared operand acquired new view")
			}
		})
	}
	var absent *JavaRef
	absent.MarkOriginalDynamicOperandDeclaration(1, JavaNull)
}

func TestOriginalNullOperandKeepsLiteralBottomTypeAndPrivateTarget(t *testing.T) {
	for _, descriptor := range []string{"Ljava/lang/Object;", "Ljava/lang/String;", "[Ljava/lang/String;", "Ljava/lang/ClassLoader;", "[I", "[[Z", "[[[Ljava/lang/String;"} {
		t.Run(descriptor, func(t *testing.T) {
			parsed, err := types.ParseMethodDescriptor("(" + descriptor + ")V")
			if err != nil {
				t.Fatal(err)
			}
			target := parsed.FunctionType().ParamTypes[0]
			seed := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			r := NewJavaRef(utils.NewRootVariableId(), seed, seed.Type().Copy())
			r.MarkOriginalDynamicOperand(9, 0, seed)
			if !r.MarkOriginalNullDynamicOperandView(9, 0, seed, target) {
				t.Fatal("missing reference target")
			}
			view, declared, known := r.OriginalDynamicOperandSourceView(seed)
			if !known || !IsNullLiteral(view) || view.String(&class_context.ClassContext{}) != "null" || func() string { n, _ := types.RawClassFQN(view.Type()); return n }() != "java.lang.Object" {
				t.Fatal("null bottom type or keyword lost")
			}
			switch raw := declared.RawType().(type) {
			case *types.JavaClass:
				raw.Name = "java.lang.Number"
			case *types.JavaPrimer:
				raw.Name = types.JavaInteger
			case *types.JavaArrayType:
				raw.Dimension++
				switch leaf := raw.JavaType.RawType().(type) {
				case *types.JavaClass:
					leaf.Name = "java.lang.Number"
				case *types.JavaPrimer:
					leaf.Name = types.JavaInteger
				}
			}

			_, again, known := r.OriginalDynamicOperandSourceView(seed)
			got, typed := SourceTypeErasure(again, &class_context.ClassContext{})
			if !known || !typed || got != descriptor {
				t.Fatal("returned type changed the private target")
			}
		})
	}
}
