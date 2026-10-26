package values

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSlotRebindRetainsSolvedDeclarationAgainstProvisionalType(t *testing.T) {
	for _, kind := range []string{"primitive array", "reference array", "generic array", "reference", "parameterized", "unsolved", "primitive coercion"} {
		t.Run(kind, func(t *testing.T) {
			var declaration types.JavaType = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
			var provisional types.JavaType = types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
			solved := true
			switch kind {
			case "reference array":
				declaration = types.NewJavaArrayType(types.NewJavaClass("java.lang.StackTraceElement"))
			case "generic array":
				declaration = types.NewJavaArrayType(types.NewJavaClass("T"))
			case "reference":
				declaration, provisional = types.NewJavaClass("example.Base"), types.NewJavaClass("example.FirstArm")
			case "parameterized":
				declaration = types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("T")})
				provisional = types.NewJavaClass("java.util.List")
			case "unsolved":
				solved = false
			case "primitive coercion":
				declaration, provisional, solved = types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaBoolean), false
			}
			ref := NewJavaRef(utils.NewRootVariableId(), nil, declaration.Copy())
			if solved {
				ref.WebDeclType = declaration.Copy()
			}
			slot := NewSlotValue(JavaNull, provisional)
			slot.ResetValue(ref)
			want := provisional
			if solved {
				want = declaration
			}
			if slot.GetValue() != ref || !reflect.DeepEqual(ref.Type().RawType(), want.RawType()) {
				t.Fatalf("rebind declaration=%s want=%s", ref.Type().String(&class_context.ClassContext{}), want.String(&class_context.ClassContext{}))
			}
			if solved && !reflect.DeepEqual(ref.WebDeclType.RawType(), declaration.RawType()) {
				t.Fatal("provisional use mutated the web's declaration")
			}
		})
	}
}

func TestForwardedSlotCannotNarrowSolvedValueOrInstruction(t *testing.T) {
	for _, kind := range []string{"object join", "interface join", "reference array", "primitive array", "parameterized", "call", "checkcast", "allocation", "unsolved", "primitive coercion"} {
		for _, depth := range []int{0, 1, 2, 8, 128, 512} {
			t.Run(fmt.Sprintf("%s/%d", kind, depth), func(t *testing.T) {
				declared := types.NewJavaClass("java.lang.Object")
				provisional := types.NewJavaClass("fixture.FirstArm")
				solved := true
				switch kind {
				case "interface join":
					declared = types.NewJavaClass("java.util.List")
				case "reference array":
					declared = types.NewJavaArrayType(types.NewJavaClass("fixture.Base"))
				case "primitive array":
					declared = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
				case "parameterized":
					declared = types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("T")})
				case "unsolved":
					solved = false
				case "primitive coercion":
					declared = types.NewJavaPrimer(types.JavaInteger)
					provisional = types.NewJavaPrimer(types.JavaBoolean)
					solved = false
				}
				ref := NewJavaRef(utils.NewRootVariableId(), nil, declared.Copy())
				if solved {
					ref.WebDeclType = declared.Copy()
				}
				var leaf JavaValue = ref
				switch kind {
				case "call":
					leaf = &FunctionCallExpression{FuncType: &types.JavaFuncType{ReturnType: declared.Copy()}}
				case "checkcast":
					leaf = &CastExpression{Value: JavaNull, TargetType: declared.Copy()}
				case "allocation":
					leaf = NewNewExpression(declared.Copy())
				}
				forwarded := leaf
				for i := 0; i < depth; i++ {
					forwarded = NewSlotValue(forwarded, provisional.Copy())
				}
				use := NewSlotValue(JavaNull, provisional.Copy())
				use.ResetValue(forwarded)
				want := declared
				if !solved {
					want = provisional
				}
				if use.GetValue() != forwarded || !reflect.DeepEqual(leaf.Type().RawType(), want.RawType()) {
					t.Fatalf("forward depth%d lost type ownership: %s want %s", depth, leaf.Type().String(&class_context.ClassContext{}), want.String(&class_context.ClassContext{}))
				}
				if solved && !reflect.DeepEqual(ref.WebDeclType.RawType(), declared.RawType()) {
					t.Fatal("changed declaration constraint")
				}
			})
		}
	}
}

func TestForwardedSlotTypePropagationIsBounded(t *testing.T) {
	for _, kind := range []string{"too deep", "cyclic", "empty", "typed nil"} {
		t.Run(kind, func(t *testing.T) {
			original := types.NewJavaClass("fixture.Original")
			ref := NewJavaRef(utils.NewRootVariableId(), nil, original.Copy())
			var forwarded JavaValue = ref
			switch kind {
			case "too deep":
				for i := 0; i < 513; i++ {
					forwarded = NewSlotValue(forwarded, original.Copy())
				}
			case "cyclic":
				a := NewSlotValue(nil, original.Copy())
				b := NewSlotValue(a, original.Copy())
				a.val = b
				forwarded = a
			case "empty":
				forwarded = NewSlotValue(nil, original.Copy())
			case "typed nil":
				forwarded = (*SlotValue)(nil)
			}
			use := NewSlotValue(JavaNull, types.NewJavaClass("fixture.Provisional"))
			use.ResetValue(forwarded)
			if use.GetValue() != forwarded || !reflect.DeepEqual(ref.Type().RawType(), original.RawType()) {
				t.Fatal("unknown forwarding granted a type update")
			}
		})
	}
}
