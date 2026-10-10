package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestScopedErasureViewRequiresLexicalExactBound(t *testing.T) {
	for _, c := range []struct {
		name, method, class, target, actual string
		want                                bool
	}{
		{"same bound", "", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.Number", true},
		{"narrowing", "", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.Object", false},
		{"different class", "", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.String", false},
		{"method shadows", "<T:Ljava/lang/Object;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.Number", false},
		{"method bound", "<T:Ljava/lang/Object;>()V", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.Object", true},
		{"unknown formal", "", "<U:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.Number", false},
		{"dependent bound", "", "<T:TU;U:Ljava/lang/Number;>Ljava/lang/Object;", "T", "java.lang.Number", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassSig: c.class, CurrentMethodSig: c.method, TypeParams: []string{"T"}}
			v := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(c.actual))
			target := types.NewJavaClass(c.target)
			if got := ScopedErasureView(ctx, target, v); got != c.want {
				t.Fatalf("got=%v want=%v", got, c.want)
			}
			if v.Type().String(ctx) != types.NewJavaClass(c.actual).String(ctx) {
				t.Fatal("mutated operand")
			}
		})
	}
	ctx := &class_context.ClassContext{ClassSig: "<T:Ljava/lang/Number;>Ljava/lang/Object;", TypeParams: []string{"T"}}
	for _, rank := range []int{1, 2, 3} {
		target := types.NewJavaClass("T")
		actual := types.NewJavaClass("java.lang.Number")
		for i := 0; i < rank; i++ {
			target = types.NewJavaArrayType(target)
			actual = types.NewJavaArrayType(actual)
		}
		v := NewJavaRef(utils.NewRootVariableId(), nil, actual)
		if bindingType(actual) != strings.Repeat("[", rank)+"Ljava/lang/Number;" {
			t.Fatal("descriptor rank", bindingType(actual))
		}
		if !ScopedErasureView(ctx, target, v) || ScopedErasureView(ctx, types.NewJavaClass("T"), v) {
			t.Fatal("array rank proof")
		}
	}
	if ScopedErasureView(ctx, types.NewJavaClass("T"), NewJavaLiteral("null", types.NewJavaClass("java.lang.Number"))) {
		t.Fatal("null needs no view")
	}
}

func TestScopedErasureViewRetainsNearestOriginalEnclosingBound(t *testing.T) {
	for _, rank := range []int{0, 1, 2, 3} {
		for _, variant := range []string{"original", "nearest shadows", "method shadows", "unknown", "dependent", "different rank", "different erasure", "too many scopes", "work", "cancelled"} {
			t.Run(strings.Repeat("array", rank)+"/"+variant, func(t *testing.T) {
				ctx := &class_context.ClassContext{TypeParams: []string{"K"}, ClassSig: "Ljava/lang/Object;", LexicalTypeParamSignatures: []string{"<K:Ljava/lang/Number;>Ljava/lang/Object;"}}
				target := types.JavaType(types.NewJavaClass("K"))
				actual := types.JavaType(types.NewJavaClass("java.lang.Number"))
				for i := 0; i < rank; i++ {
					target = types.NewJavaArrayType(target)
					actual = types.NewJavaArrayType(actual)
				}
				switch variant {
				case "nearest shadows":
					ctx.ClassSig = "<K:Ljava/lang/Object;>Ljava/lang/Object;"
				case "method shadows":
					ctx.CurrentMethodSig = "<K:Ljava/lang/Object;>()V"
				case "unknown":
					ctx.LexicalTypeParamSignatures = nil
				case "dependent":
					ctx.LexicalTypeParamSignatures = []string{"<K:TU;U:Ljava/lang/Number;>Ljava/lang/Object;"}
				case "different rank":
					actual = types.NewJavaArrayType(actual)
				case "different erasure":
					actual = types.NewJavaClass("java.lang.Object")
				case "too many scopes":
					ctx.LexicalTypeParamSignatures = make([]string, 129)
				case "work":
					ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "cancelled":
					c, cancel := context.WithCancel(context.Background())
					cancel()
					ctx.Work = workbudget.New(c, workbudget.Limits{})
				}
				v := NewJavaRef(utils.NewRootVariableId(), nil, actual)
				if got := ScopedErasureView(ctx, target, v); got != (variant == "original") {
					t.Fatalf("erasure view=%v", got)
				}
			})
		}
	}
}
