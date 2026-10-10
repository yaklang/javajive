package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFunctionalTargetCannotSpellCaptureAsWildcardBound(t *testing.T) {
	ctx := &class_context.ClassContext{TypeParams: []string{"T"}}
	for _, variance := range []string{"extends", "super"} {
		for _, captured := range []types.JavaType{
			&types.JavaWildcardType{},
			&types.JavaWildcardType{Variant: "extends", Bound: types.NewJavaClass("java.lang.Number")},
			&types.JavaWildcardType{Variant: "super", Bound: types.NewJavaClass("java.lang.Integer")},
		} {
			target := types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{&types.JavaWildcardType{Variant: variance, Bound: captured}})
			if sourceDenotableJavaType(target, ctx) {
				t.Fatalf("capture cannot be printed as %s", target.String(ctx))
			}
		}
		for _, reference := range []types.JavaType{
			types.NewJavaClass("T"), types.NewJavaClass("java.lang.Number"),
			types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)),
			types.NewParameterizedType("java.util.List", []types.JavaType{&types.JavaWildcardType{Variant: "extends", Bound: types.NewJavaClass("T")}}),
		} {
			target := types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{&types.JavaWildcardType{Variant: variance, Bound: reference}})
			if !sourceDenotableJavaType(target, ctx) {
				t.Fatalf("legal reference bound lost: %s", target.String(ctx))
			}
		}
	}
	for _, malformed := range []*types.JavaWildcardType{
		nil, {Variant: "extends"}, {Variant: "super"}, {Variant: "other", Bound: types.NewJavaClass("T")},
		{Bound: types.NewJavaClass("T")}, {Variant: "super", Bound: types.NewJavaPrimer(types.JavaInteger)},
	} {
		if sourceDenotableJavaType(malformed, ctx) {
			t.Fatal("malformed or primitive wildcard bound accepted", malformed)
		}
	}
}
