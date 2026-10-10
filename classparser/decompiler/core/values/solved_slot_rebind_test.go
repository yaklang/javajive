package values

import (
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
