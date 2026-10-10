package statements

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestConditionalGenericAssignmentRequiresEveryOriginalErasureAndFixedResult(t *testing.T) {
	for _, scenario := range []string{"proved", "reverse", "nested null", "direct class formal", "inferred formal subtype", "ordinary class named formal", "method formal shadows class", "different formal bound", "unbounded formal", "formal bound chain", "formal array", "missing metadata", "incomplete parents", "foreign identity", "unrelated result", "narrowing", "missing declaration", "missing method", "method formal", "wrong signature erasure", "bridge", "static", "special", "missing origin", "poly input", "wildcard target", "array target", "wrong field descriptor", "missing field signature", "no hidden generic result", "missing arm", "depth", "nodes", "budget", "canceled", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "()Lproof/Product;"
			meta := map[string]callbinding.Class{
				"proof/Producer": {Name: "proof/Producer", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "finish", Desc: desc}}},
				"proof/Product":  {Name: "proof/Product", MembersComplete: true, ParentsComplete: true, Parents: []string{"proof/Lookup"}},
				"proof/Lookup":   {Name: "proof/Lookup", MembersComplete: true, ParentsComplete: true, IsInterface: true},
			}
			methodSig := "()Lproof/Product<TE;>;"
			classSig := "<E:Ljava/lang/Object;>Ljava/lang/Object;"
			ctx := &class_context.ClassContext{ClassName: "proof.Owner", FieldSignatures: map[string]string{"registry": "Lproof/Lookup<Lproof/Entry;>;"}}
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) { c, ok := meta[n]; return c, ok }
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				return classSig, map[string]string{class_context.MethodDescKey("finish", desc): methodSig}, n == "proof/Producer"
			}
			self := ternaryCastTestRef("self", "proof.Owner")
			self.IsThis = true
			left := values.NewRefMember(self, "registry", types.NewJavaClass("proof.Lookup"))
			arm := ternaryCastTestRef("provided", "proof.Lookup")
			call := &values.FunctionCallExpression{ClassName: "proof.Producer", FunctionName: "finish", Descriptor: desc, Kind: values.InvokeVirtual, HasOriginPC: true, OriginPC: 31, Object: ternaryCastTestRef("producer", "proof.Producer"), FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("proof.Product")}}
			condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			var rhs values.JavaValue = values.NewTernaryExpression(condition, arm, call)
			switch scenario {
			case "reverse":
				rhs = values.NewTernaryExpression(condition, call, arm)
			case "nested null":
				rhs = values.NewTernaryExpression(condition, rhs, values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")))
			case "direct class formal", "inferred formal subtype", "ordinary class named formal", "method formal shadows class", "different formal bound", "unbounded formal", "formal bound chain", "formal array":
				classSig = "<E:Ljava/lang/Object;R:Lproof/Product<TE;>;>Ljava/lang/Object;"
				methodSig = "()TR;"
				switch scenario {
				case "inferred formal subtype":
					// The physical return is Lookup while the receiver's recovered
					// substitution is Product. Descriptor evidence must control it.
					desc = "()Lproof/Lookup;"
					call.Descriptor = desc
					c := meta["proof/Producer"]
					c.Methods[0].Desc = desc
					meta["proof/Producer"] = c
					classSig = "<E:Ljava/lang/Object;R:Lproof/Lookup<TE;>;>Ljava/lang/Object;"
				case "ordinary class named formal":
					methodSig = "()LR;"
				case "method formal shadows class":
					methodSig = "<R:Lproof/Product<TE;>;>()TR;"
				case "different formal bound":
					classSig = "<E:Ljava/lang/Object;R:Lproof/Lookup<TE;>;>Ljava/lang/Object;"
				case "unbounded formal":
					classSig = "<E:Ljava/lang/Object;R:Ljava/lang/Object;>Ljava/lang/Object;"
				case "formal bound chain":
					classSig = "<E:Ljava/lang/Object;R:TE;>Ljava/lang/Object;"
				case "formal array":
					methodSig = "()[TR;"
				}
			case "missing metadata":
				delete(meta, "proof/Product")
			case "incomplete parents":
				c := meta["proof/Product"]
				c.ParentsComplete = false
				meta["proof/Product"] = c
			case "foreign identity":
				c := meta["proof/Product"]
				c.Name = "proof/Foreign"
				meta["proof/Product"] = c
			case "unrelated result":
				c := meta["proof/Product"]
				c.Parents = nil
				meta["proof/Product"] = c
			case "narrowing":
				c := meta["proof/Product"]
				c.Parents = nil
				meta["proof/Product"] = c
				c = meta["proof/Lookup"]
				c.Parents = []string{"proof/Product"}
				meta["proof/Lookup"] = c
			case "missing declaration":
				ctx.SiblingClassSig = nil
			case "missing method":
				c := meta["proof/Producer"]
				c.Methods = nil
				meta["proof/Producer"] = c
			case "method formal":
				methodSig = "<M:Ljava/lang/Object;>()Lproof/Product<TM;>;"
			case "wrong signature erasure":
				methodSig = "()Lproof/Lookup<TE;>;"
			case "bridge":
				c := meta["proof/Producer"]
				c.Methods[0].Bridge = true
				meta["proof/Producer"] = c
			case "static":
				call.IsStatic = true
			case "special":
				call.IsSpecialInvoke = true
			case "missing origin":
				call.HasOriginPC = false
			case "poly input":
				call.Arguments = []values.JavaValue{rhs}
			case "wildcard target":
				ctx.FieldSignatures["registry"] = "Lproof/Lookup<*>;"
			case "array target":
				ctx.FieldSignatures["registry"] = "[Lproof/Lookup<Lproof/Entry;>;"
			case "wrong field descriptor":
				left = values.NewRefMember(self, "registry", types.NewJavaClass("java.lang.Object"))
			case "missing field signature":
				ctx.FieldSignatures = nil
			case "no hidden generic result":
				rhs = values.NewTernaryExpression(condition, arm, arm)
			case "missing arm":
				rhs = values.NewTernaryExpression(condition, arm, nil)
			case "depth":
				for i := 0; i < 34; i++ {
					rhs = values.NewTernaryExpression(condition, arm, rhs)
				}
			case "nodes":
				for i := 0; i < 9; i++ {
					rhs = values.NewTernaryExpression(condition, rhs, rhs)
				}
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			case "disabled":
				ctx.Env = func(k string) string {
					if k == "JDEC_CONDITIONAL_GENERIC_ASSIGNMENT_OFF" {
						return "1"
					}
					return ""
				}
			}
			before := call.Witness()
			want := scenario == "proved" || scenario == "reverse" || scenario == "nested null" || scenario == "direct class formal" || scenario == "inferred formal subtype"
			got := conditionalGenericAssignmentBridge(ctx, left, rhs)
			if (got != "") != want {
				t.Fatalf("bridge=%q expected=%v", got, want)
			}
			if call.Witness() != before {
				t.Fatal("original invocation changed")
			}
		})
	}
}
