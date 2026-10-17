package types

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestRawAllocationSupertypeKeepsDeclaredInheritanceRules(t *testing.T) {
	for _, mode := range []string{"raw edge", "raw chain", "interface physical superclass", "duplicate interface superclass", "raw diamond", "fixed nongeneric edge", "fixed nongeneric chain", "raw drops fixed argument", "conflicting diamond", "solved allocation", "missing class", "wrong identity", "incomplete parents", "wrong physical edge", "duplicate edge", "free formal", "malformed signature", "cycle", "typed nil", "unrelated", "work", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			metadata := map[string]callbinding.Class{
				"x/Leaf":           {Name: "x/Leaf", Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TT;>;", Parents: []string{"java/lang/Object", "x/Target"}, ParentsComplete: true},
				"x/Target":         {Name: "x/Target", Signature: "<E:Ljava/lang/Object;>Ljava/lang/Object;", ParentsComplete: true, IsInterface: true},
				"java/lang/Object": {Name: "java/lang/Object", ParentsComplete: true},
			}
			ctx := &class_context.ClassContext{}
			var source JavaType = NewJavaClass("x.Leaf")
			leaf := metadata["x/Leaf"]
			want, instantiated := false, false
			switch mode {
			case "raw edge":
				want = true
			case "raw drops fixed argument":
				leaf.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<Ljava/lang/String;>;"
				want = true
			case "raw chain", "interface physical superclass", "duplicate interface superclass":
				leaf.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;Lx/Middle<TT;>;"
				leaf.Parents = []string{"java/lang/Object", "x/Middle"}
				metadata["x/Middle"] = callbinding.Class{Name: "x/Middle", Signature: "<M:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TM;>;", Parents: []string{"x/Target"}, ParentsComplete: true, IsInterface: true}
				want = true
				if mode != "raw chain" {
					m := metadata["x/Middle"]
					m.Parents = append(m.Parents, "java/lang/Object")
					if mode == "duplicate interface superclass" {
						m.Parents = append(m.Parents, "java/lang/Object")
						want = false
					}
					metadata[m.Name] = m
				}
			case "fixed nongeneric edge", "fixed nongeneric chain":
				leaf.Signature = "Ljava/lang/Object;Lx/Target<Ljava/lang/String;>;"
				if mode == "fixed nongeneric chain" {
					leaf.Signature = "Ljava/lang/Object;Lx/Middle<Ljava/lang/String;>;"
					leaf.Parents = []string{"java/lang/Object", "x/Middle"}
					metadata["x/Middle"] = callbinding.Class{Name: "x/Middle", Signature: "<M:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TM;>;", Parents: []string{"x/Target"}, ParentsComplete: true, IsInterface: true}
				}
				want, instantiated = true, true
			case "raw diamond", "conflicting diamond":
				leaf.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;Lx/Left<TT;>;Lx/Right<TT;>;"
				leaf.Parents = []string{"java/lang/Object", "x/Left", "x/Right"}
				for _, n := range []string{"x/Left", "x/Right"} {
					metadata[n] = callbinding.Class{Name: n, Signature: "<A:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TA;>;", Parents: []string{"x/Target"}, ParentsComplete: true, IsInterface: true}
				}
				want = mode == "raw diamond"
				if !want {
					// A nongeneric declaration retains its fixed edge while the
					// other branch is raw. These source views cannot be merged.
					metadata["x/Right"] = callbinding.Class{Name: "x/Right", Signature: "Ljava/lang/Object;Lx/Target<Ljava/lang/String;>;", Parents: []string{"x/Target"}, ParentsComplete: true, IsInterface: true}
				}
			case "solved allocation":
				source = NewParameterizedType("x.Leaf", []JavaType{NewJavaClass("java.lang.String")})
			case "missing class":
				delete(metadata, "x/Target")
			case "wrong identity":
				leaf.Name = "foreign/Leaf"
			case "incomplete parents":
				leaf.ParentsComplete = false
			case "wrong physical edge":
				leaf.Parents = []string{"java/lang/Object", "x/Other"}
			case "duplicate edge":
				leaf.Parents = []string{"x/Target", "x/Target"}
			case "free formal":
				leaf.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;Lx/Target<TU;>;"
			case "malformed signature":
				leaf.Signature = "garbage"
			case "cycle":
				leaf.Signature = "<T:Ljava/lang/Object;>Lx/Leaf<TT;>;"
				leaf.Parents = []string{"x/Leaf"}
			case "typed nil":
				source = (*JavaTypeWrap)(nil)
			case "unrelated":
				leaf.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				leaf.Parents = []string{"java/lang/Object"}
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			metadata["x/Leaf"] = leaf
			got, ok := ProjectRawAllocationSupertype(ctx, source, "x/Target", func(n string) (callbinding.Class, bool) { m, k := metadata[n]; return m, k })
			if ok != want {
				t.Fatalf("projection=%v want=%v", ok, want)
			}
			if !ok {
				return
			}
			p, parameterized := AsParameterizedType(got)
			if parameterized != instantiated {
				t.Fatalf("instantiated=%v want=%v", parameterized, instantiated)
			}
			if instantiated && (len(p.TypeArgs) != 1 || p.TypeArgs[0].String(ctx) != "String") {
				t.Fatal("fixed nongeneric argument lost", got)
			}
			if raw, known := RawClassFQN(got); !known || raw != "x.Target" {
				t.Fatal("wrong original supertype", got)
			}
		})
	}
}
