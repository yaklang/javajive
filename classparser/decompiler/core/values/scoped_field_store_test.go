package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestScopedFieldStoreRequiresOriginalSelectionAndSourceBinder(t *testing.T) {
	for _, rank := range []int{0, 1, 2} {
		for _, variant := range []string{"exact", "projected", "missing projection", "raw", "missing field", "wrong field sig", "missing anchor", "read anchor", "wrong PC", "wrong owner", "wrong descriptor", "unknown binder", "method shadow", "narrow operand", "null", "work", "cancelled"} {
			t.Run(strings.Repeat("array", rank)+"/"+variant, func(t *testing.T) {
				ctx := &class_context.ClassContext{ClassSig: "<R:Ljava/lang/Number;>Ljava/lang/Object;", TypeParams: []string{"R"}}
				classSig := "<E:Ljava/lang/Number;>Ljava/lang/Object;"
				fieldSig := strings.Repeat("[", rank) + "TE;"
				if variant == "projected" || variant == "missing projection" {
					classSig = "Ljava/lang/Object;"
				}
				ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) { return classSig, nil, n == "example/Cell" }
				ctx.SiblingFieldSig = func(n, f string) (string, bool) {
					return fieldSig, n == "example/Cell" && f == "value" && variant != "missing field"
				}
				if variant == "projected" {
					ctx.SiblingSourceClassFormals = func(n string) ([]string, bool) { return []string{"E"}, n == "example/Cell" }
				}
				receiverType := types.NewParameterizedType("example.Cell", []types.JavaType{types.NewJavaTypeVariable("R")})
				if variant == "raw" {
					receiverType = types.NewJavaClass("example.Cell")
				}
				receiver := NewJavaRef(nil, nil, receiverType)
				actual := types.JavaType(types.NewJavaClass("java.lang.Number"))
				for n := 0; n < rank; n++ {
					actual = types.NewJavaArrayType(actual)
				}
				field := NewRefMember(receiver, "value", actual)
				member := NewJavaClassMember("example.Cell", "value", bindingType(actual), actual)
				value := JavaValue(NewJavaRef(nil, nil, actual))
				if variant == "read anchor" {
					field.OriginPC, field.HasOriginPC = 7, true
					field.MarkOriginalFieldRead(member, 7)
				} else if variant != "missing anchor" {
					field.MarkOriginalFieldWrite(member, 7)
				}
				switch variant {
				case "wrong field sig":
					fieldSig = "Ljava/lang/String;"
				case "wrong PC":
					field.OriginPC++
				case "wrong owner":
					field.Object = NewJavaRef(nil, nil, types.NewParameterizedType("example.Other", []types.JavaType{types.NewJavaTypeVariable("R")}))
				case "wrong descriptor":
					field.JavaType = types.NewJavaClass("java.lang.Object")
				case "unknown binder":
					ctx.ClassSig = "Ljava/lang/Object;"
				case "method shadow":
					ctx.CurrentMethodSig = "<R:Ljava/lang/Number;>()V"
				case "narrow operand":
					value = NewJavaRef(nil, nil, types.NewJavaClass("java.lang.Object"))
				case "null":
					value = NewJavaLiteral("null", actual)
				case "work":
					ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "cancelled":
					base, cancel := context.WithCancel(context.Background())
					cancel()
					ctx.Work = workbudget.New(base, workbudget.Limits{})
				}
				got := ScopedFieldStoreView(ctx, field, value)
				want := variant == "exact" || variant == "projected"
				if (got != nil) != want {
					t.Fatalf("view=%v want=%v", got, want)
				}
				if got != nil {
					if got.String(ctx) != "R"+strings.Repeat("[]", rank) {
						t.Fatal("lost array rank or binder")
					}
				}
			})
		}
	}
}
