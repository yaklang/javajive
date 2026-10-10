package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestClassLiteralSubjectIsIndependentFromConsumerTypeViews(t *testing.T) {
	for _, typ := range []types.JavaType{types.NewJavaClass("example.Owner"), types.NewJavaClass("java.lang.String"), types.NewJavaPrimer(types.JavaInteger), types.NewJavaArrayType(types.NewJavaClass("example.Item")), types.NewJavaArrayType(types.NewJavaPrimer(types.JavaBoolean))} {
		t.Run(typ.String(&class_context.ClassContext{}), func(t *testing.T) {
			v := NewJavaClassValue(typ)
			v.OriginPC, v.HasOriginPC = 19, true
			ctx := &class_context.ClassContext{}
			before := v.String(ctx)
			for _, consumer := range []types.JavaType{types.NewJavaClass("java.lang.Class"), types.NewJavaClass("java.lang.Object"), types.NewJavaClass("other.Subject")} {
				view := v.Type()
				view.ResetType(consumer)
				if v.String(ctx) != before || v.OriginPC != 19 || !v.HasOriginPC {
					t.Fatal("original literal subject/origin changed")
				}
			}
		})
	}
	var absent *JavaClassValue
	if absent.Type() != nil || NewJavaClassValue(nil).Type() != nil {
		t.Fatal("invented missing literal subject")
	}
}
