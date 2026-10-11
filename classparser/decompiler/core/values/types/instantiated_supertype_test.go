package types

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestInstantiatedSupertypeRequiresConsistentCompleteHierarchy(t *testing.T) {
	for _, mode := range []string{"single edge", "two edges", "diamond", "conflicting diamond", "missing class", "wrong name", "incomplete parents", "wrong physical edge", "duplicate physical edge", "free formal", "raw generic", "wrong arity", "wildcard", "primitive argument", "cyclic type", "typed nil type", "deep type", "owner segments", "malformed Signature", "cycle", "unrelated", "work", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			metadata := map[string]callbinding.Class{
				"x/Leaf":           {Name: "x/Leaf", Signature: "<X:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TX;>;", Parents: []string{"java/lang/Object", "x/Target"}, ParentsComplete: true},
				"x/Target":         {Name: "x/Target", Signature: "<E:Ljava/lang/Object;>Ljava/lang/Object;", ParentsComplete: true, IsInterface: true},
				"java/lang/Object": {Name: "java/lang/Object", ParentsComplete: true},
			}
			ctx := &class_context.ClassContext{}
			source := NewParameterizedType("x.Leaf", []JavaType{NewJavaArrayType(NewJavaClass("java.lang.String"))})
			change := metadata["x/Leaf"]
			switch mode {
			case "two edges":
				change.Signature = "<X:Ljava/lang/Object;>Ljava/lang/Object;Lx/Middle<TX;>;"
				change.Parents = []string{"java/lang/Object", "x/Middle"}
				metadata["x/Middle"] = callbinding.Class{Name: "x/Middle", Signature: "<M:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TM;>;", Parents: []string{"x/Target"}, ParentsComplete: true, IsInterface: true}
			case "diamond", "conflicting diamond":
				change.Signature = "<X:Ljava/lang/Object;>Ljava/lang/Object;Lx/Left<TX;>;Lx/Right<TX;>;"
				change.Parents = []string{"java/lang/Object", "x/Left", "x/Right"}
				for _, n := range []string{"x/Left", "x/Right"} {
					metadata[n] = callbinding.Class{Name: n, Signature: "<A:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TA;>;", Parents: []string{"x/Target"}, ParentsComplete: true, IsInterface: true}
				}
				if mode == "conflicting diamond" {
					m := metadata["x/Right"]
					m.Signature = "<A:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<Ljava/lang/Object;>;"
					metadata[m.Name] = m
				}
			case "missing class":
				delete(metadata, "x/Target")
			case "wrong name":
				change.Name = "foreign/Leaf"
			case "incomplete parents":
				change.ParentsComplete = false
			case "wrong physical edge":
				change.Parents = []string{"java/lang/Object", "x/Other"}
			case "duplicate physical edge":
				change.Parents = []string{"x/Target", "x/Target"}
			case "free formal":
				change.Signature = "<X:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TY;>;"
			case "raw generic":
				source = NewJavaClass("x.Leaf")
			case "wrong arity":
				source = NewParameterizedType("x.Leaf", nil)
			case "wildcard":
				source = NewParameterizedType("x.Leaf", []JavaType{&JavaWildcardType{Variant: "+", Bound: NewJavaClass("java.lang.String")}})
			case "primitive argument":
				source = NewParameterizedType("x.Leaf", []JavaType{NewJavaPrimer(JavaInteger)})
			case "cyclic type":
				p, _ := AsParameterizedType(source)
				p.TypeArgs = []JavaType{source}
			case "typed nil type":
				source = (*JavaTypeWrap)(nil)

			case "deep type":
				var typ JavaType = NewJavaClass("java.lang.String")
				for i := 0; i < 34; i++ {
					typ = NewParameterizedType("x.Box", []JavaType{typ})
				}
				source = NewParameterizedType("x.Leaf", []JavaType{typ})

			case "owner segments":
				p, _ := AsParameterizedType(source)
				p.OwnerSegments = []NestedTypeSegment{{BinaryName: "x.Owner"}, {BinaryName: "x.Leaf", TypeArgs: p.TypeArgs}}
			case "malformed Signature":
				change.Signature = "garbage"
			case "cycle":
				change.Signature = "<X:Ljava/lang/Object;>Lx/Leaf<TX;>;"
				change.Parents = []string{"x/Leaf"}
			case "unrelated":
				change.Signature = "<X:Ljava/lang/Object;>Ljava/lang/Object;"
				change.Parents = []string{"java/lang/Object"}
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			metadata["x/Leaf"] = change
			got, ok := ProjectInstantiatedSupertype(ctx, source, "x/Target", func(n string) (callbinding.Class, bool) { m, k := metadata[n]; return m, k })
			want := mode == "single edge" || mode == "two edges" || mode == "diamond"
			if ok != want {
				t.Fatalf("projection=%v want=%v", ok, want)
			}
			if ok {
				p, k := AsParameterizedType(got)
				if !k || p.RawClassName != "x.Target" || len(p.TypeArgs) != 1 || !p.TypeArgs[0].IsArray() || p.TypeArgs[0].ElementType().String(ctx) != "String" {
					t.Fatal("composed invariant array argument", got)
				}
			}
		})
	}
}
