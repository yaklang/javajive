package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"
)

func TestZeroInputFactoryResultRequiresClosedUniqueBinding(t *testing.T) {
	for _, scenario := range []string{"proved", "direct", "no metadata", "missing ancestor", "incomplete family", "duplicate return", "bridge", "varargs", "wrong static declaration", "missing signature", "trailing signature", "malformed formal", "malformed result argument", "free receiver formal", "generic throws", "free leaf formal", "dependent leaf bound", "missing origin", "static kind mismatch", "interface kind mismatch", "reserved method", "special", "dynamic", "nil receiver", "foreign receiver", "argument", "malformed descriptor", "array result", "primitive result", "wrong result", "nongeneric leaf", "too deep"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "()Lproof/Order;"
			signatures := map[string]string{
				class_context.MethodDescKey("begin", desc): "<C::Ljava/lang/Comparable;>()Lproof/Order<TC;>;",
				class_context.MethodDescKey("back", desc):  "<S:TX;>()Lproof/Order<TS;>;",
			}
			meta := map[string]callbinding.Class{
				"proof/Order":      {Name: "proof/Order", Public: true, Parents: []string{"java/lang/Object"}, ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "begin", Desc: desc, Static: true, Generic: true}, {Name: "back", Desc: desc, Generic: true}}},
				"java/lang/Object": {Name: "java/lang/Object", Public: true, ParentsComplete: true, MembersComplete: true},
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) { c, ok := meta[name]; return c, ok }, SiblingClassSig: func(name string) (string, map[string]string, bool) {
				return "<X:Ljava/lang/Object;>Ljava/lang/Object;", signatures, name == "proof/Order"
			}}
			leaf := &FunctionCallExpression{ClassName: "proof.Order", FunctionName: "begin", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, OriginPC: 7, HasOriginPC: true}
			outer := &FunctionCallExpression{ClassName: "proof.Order", FunctionName: "back", Descriptor: desc, Object: leaf, Kind: InvokeVirtual, OriginPC: 11, HasOriginPC: true}
			value := outer
			result := "Lproof/Order;"
			owner := meta["proof/Order"]
			switch scenario {
			case "direct":
				value = leaf
			case "no metadata":
				ctx.InvocationMetadata = nil
			case "missing ancestor":
				delete(meta, "java/lang/Object")
			case "incomplete family":
				owner.MembersComplete = false
			case "duplicate return":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "begin", Desc: "()Ljava/lang/Object;", Static: true})
			case "bridge":
				owner.Methods[0].Bridge = true
			case "varargs":
				owner.Methods[0].Varargs = true
			case "wrong static declaration":
				owner.Methods[0].Static = false
			case "missing signature":
				delete(signatures, class_context.MethodDescKey("begin", desc))
			case "trailing signature":
				signatures[class_context.MethodDescKey("begin", desc)] += "junk"
			case "malformed formal":
				signatures[class_context.MethodDescKey("back", desc)] = "<S:>()Lproof/Order<TS;>;"
			case "malformed result argument":
				signatures[class_context.MethodDescKey("back", desc)] = "<S:TX;>()Lproof/Order<garbage>;"
			case "free receiver formal":
				signatures[class_context.MethodDescKey("back", desc)] = "<S:TZ;>()Lproof/Order<TS;>;"
			case "generic throws":
				signatures[class_context.MethodDescKey("begin", desc)] += "^TC;"
			case "free leaf formal":
				signatures[class_context.MethodDescKey("begin", desc)] = "<C:Ljava/lang/Object;>()Lproof/Order<TZ;>;"
			case "dependent leaf bound":
				signatures[class_context.MethodDescKey("begin", desc)] = "<C:TZ;>()Lproof/Order<TC;>;"
			case "missing origin":
				leaf.HasOriginPC = false
			case "static kind mismatch":
				leaf.Kind = InvokeVirtual
			case "interface kind mismatch":
				outer.Kind = InvokeInterface
			case "reserved method":
				leaf.FunctionName = "<clinit>"
			case "special":
				outer.Kind = InvokeSpecial
			case "dynamic":
				outer.Kind = InvokeDynamic
			case "nil receiver":
				outer.Object = nil
			case "foreign receiver":
				leaf.Descriptor = "()Lproof/Other;"
			case "argument":
				leaf.Arguments = []JavaValue{JavaNull}
			case "malformed descriptor":
				leaf.Descriptor = "junk"
			case "array result":
				outer.Descriptor = "()[Lproof/Order;"
				result = "[Lproof/Order;"
			case "primitive result":
				outer.Descriptor = "()I"
				result = "I"
			case "wrong result":
				result = "Ljava/lang/Object;"
			case "nongeneric leaf":
				owner.Methods[0].Generic = false
				signatures[class_context.MethodDescKey("begin", desc)] = "()Lproof/Order;"
			case "too deep":
				for i := 0; i < 32; i++ {
					copy := *outer
					copy.Object = value
					value = &copy
				}
			}
			meta["proof/Order"] = owner
			before := outer.Witness()
			leafBefore := leaf.Witness()
			got := ErasedZeroInputFactoryResult(ctx, value, result)
			if got != (scenario == "proved" || scenario == "direct") {
				t.Fatalf("proof=%v", got)
			}
			if outer.Witness() != before || leaf.Witness() != leafBefore || leaf.Arguments != nil && scenario != "argument" {
				t.Fatal("proof mutated invocation identity")
			}
		})
	}
}

func TestZeroFactorySignatureConsumesEntireClosedGrammar(t *testing.T) {
	for _, sig := range []string{"<T:Ljava/lang/Object;>()Lprobe/Box<TT;>;", "<T::Ljava/lang/Comparable<TT;>;>()Lprobe/Box<+TT;>;", "<S:TX;>()Lprobe/Box<TS;>;", "()Lprobe/Outer<TX;>.Inner<*>;", "()Lprobe/Box<[Ljava/lang/Object;>;"} {
		if !closedZeroInputReferenceSignature(sig) {
			t.Fatalf("valid signature rejected: %s", sig)
		}
	}
	for _, sig := range []string{"<>()Lprobe/Box;", "<T:>()Lprobe/Box<TT;>;", "()Lprobe/Box<>;", "()Lprobe/Box<garbage>;", "()Lprobe/Box<T;>;", "()Lprobe/Box<+>;", "()Lprobe/Box<[[V>;", "()Lprobe/Box;extra", "()Lprobe/Box;^TT;", "()TT;", "()L;", "()Lprobe/Outer..Inner;"} {
		if closedZeroInputReferenceSignature(sig) {
			t.Fatalf("malformed or unsupported signature accepted: %s", sig)
		}
	}
}
