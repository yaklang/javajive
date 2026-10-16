package types

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestSourceBridgeParameterViewPreservesPhysicalInputErasure(t *testing.T) {
	for _, scenario := range []string{"original", "method shadows enclosing bound", "dependent Object bound", "competing arity", "no proof", "wrong parent", "self target", "raw receiver", "missing argument", "wrong arity", "arity only", "ordinary override", "unknown parent", "foreign formal", "concrete narrowing", "bounded narrowing", "outer narrowing", "missing caller declaration", "callee method inference", "unknown caller bound", "cyclic caller bound", "work", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			const descriptor = "(Ljava/lang/Object;)Z"
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, ClassSig: "<T:Ljava/lang/Object;>Ljava/lang/Object;"}
			target := "probe/Parent"
			if scenario == "wrong parent" {
				target = "probe/Other"
			}
			if scenario == "self target" {
				target = "probe/Child"
			}
			ctx.SourceBridgeTarget = func(owner, name, desc string) (string, bool) {
				return target, owner == "probe/Child" && name == "apply" && desc == descriptor && scenario != "ordinary override"
			}
			if scenario == "no proof" {
				ctx.SourceBridgeTarget = nil
			}
			provider := func(owner string) (string, map[string]string, bool) {
				switch owner {
				case "probe/Child":
					methods := map[string]string{class_context.MethodDescKey("apply", descriptor): ""}
					if scenario == "competing arity" {
						methods[class_context.MethodSigKey("apply", 1)] = "(Ljava/lang/String;)Z"
					}
					return "<V:Ljava/lang/Object;>Lprobe/Parent<TV;>;", methods, true
				case "probe/Parent":
					sig := "(TX;)Z"
					if scenario == "callee method inference" {
						sig = "<T:Ljava/lang/Object;>(TT;)Z"
					}
					return "<X:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", descriptor): sig}, scenario != "unknown parent"
				}
				return "", nil, false
			}
			args := []JavaType{NewJavaClass("T")}
			argc := 1
			switch scenario {
			case "method shadows enclosing bound":
				ctx.ClassSig = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				ctx.CurrentMethodSig = "<T:Ljava/lang/Object;>()V"
			case "dependent Object bound":
				ctx.ClassSig = "<T:TU;U:Ljava/lang/Object;>Ljava/lang/Object;"
			case "raw receiver":
				args = nil
			case "missing argument":
				args = []JavaType{nil}
			case "wrong arity":
				argc = 2
			case "foreign formal":
				args = []JavaType{NewJavaClass("Foreign")}
			case "concrete narrowing":
				args = []JavaType{NewJavaClass("java.lang.String")}
			case "bounded narrowing":
				ctx.ClassSig = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
			case "outer narrowing":
				ctx.ClassSig = "Ljava/lang/Object;"
				ctx.LexicalTypeParamSignatures = []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}
			case "missing caller declaration":
				ctx.ClassSig = "Ljava/lang/Object;"
			case "unknown caller bound":
				ctx.ClassSig = "<T:TU;>Ljava/lang/Object;"
			case "cyclic caller bound":
				ctx.ClassSig = "<T:TU;U:TT;>Ljava/lang/Object;"
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			var ps []JavaType
			if scenario == "arity only" {
				ps, _ = ResolveInstantiatedSignature(ctx, provider, "probe.Child", args, "apply", argc)
			} else {
				ps, _, _ = ResolveInstantiatedSignatureExact(ctx, provider, "probe.Child", args, "apply", descriptor, argc)
			}
			want := scenario == "original" || scenario == "method shadows enclosing bound" || scenario == "dependent Object bound" || scenario == "competing arity"
			if want {
				if len(ps) != 1 || ps[0] == nil || ps[0].String(&class_context.ClassContext{}) != "T" {
					t.Fatalf("original erased input not restored: %v", ps)
				}
			} else if len(ps) != 0 {
				t.Fatalf("unsafe signature view accepted: %v", ps)
			}
		})
	}
}

func TestSourceBridgeParameterErasureRequiresCompleteBoundEnvironment(t *testing.T) {
	for _, variant := range []string{"same bound", "array", "primitive", "concrete", "different bound", "method shadow", "nearest shadow", "unknown", "cycle", "depth", "scope length", "scope count", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, ClassSig: "Ljava/lang/Object;", LexicalTypeParamSignatures: []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}}
			target := JavaType(NewJavaClass("T"))
			want := "Ljava/lang/Number;"
			good := true
			switch variant {
			case "array":
				target = NewJavaArrayType(NewJavaArrayType(target))
				want = "[[Ljava/lang/Number;"
			case "primitive":
				target = NewJavaPrimer("int")
				want = "primitive:int"
			case "concrete":
				target = NewJavaClass("java.lang.String")
				want = "Ljava/lang/String;"
			case "different bound":
				ctx.LexicalTypeParamSignatures = []string{"<T:Ljava/lang/Object;>Ljava/lang/Object;"}
				want = "Ljava/lang/Object;"
			case "method shadow":
				ctx.CurrentMethodSig = "<T:Ljava/lang/Object;>()V"
				want = "Ljava/lang/Object;"
			case "nearest shadow":
				ctx.ClassSig = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				want = "Ljava/lang/Object;"
			case "unknown":
				ctx.LexicalTypeParamSignatures = nil
				good = false
			case "cycle":
				ctx.LexicalTypeParamSignatures = []string{"<T:TU;U:TT;>Ljava/lang/Object;"}
				good = false
			case "depth":
				for i := 0; i < 66; i++ {
					target = NewJavaArrayType(target)
				}
				good = false
			case "scope length":
				ctx.LexicalTypeParamSignatures = []string{strings.Repeat("x", 65536)}
				good = false
			case "scope count":
				ctx.LexicalTypeParamSignatures = make([]string, 129)
				good = false
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				good = false
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				good = false
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
				good = false
			}
			ctx.DeclarationSourceName = func(string) (string, bool) { t.Fatal("erasure query formatted source"); return "", false }
			got, known := sourceBridgeParameterErasure(ctx, target, 0)
			if known != good || known && got != want {
				t.Fatalf("erasure=%q/%v want=%q/%v", got, known, want, good)
			}
		})
	}
}

// Return contexts can also add an implicit CHECKCAST after an erased invoke.
// The declaration-only bridge query has no consumer witness to license one.
func TestSourceBridgeResultViewCannotInventPayloadCheck(t *testing.T) {
	for _, input := range []string{"", "I"} {
		for _, bound := range []string{"java/lang/Object", "java/lang/Number"} {
			t.Run(input+"/"+bound, func(t *testing.T) {
				desc := "(" + input + ")Ljava/lang/Object;"
				ctx := &class_context.ClassContext{TypeParams: []string{"T"}, ClassSig: "<T:L" + bound + ";>Ljava/lang/Object;"}
				ctx.SourceBridgeTarget = func(owner, name, d string) (string, bool) {
					return "probe/Parent", owner == "probe/Child" && name == "get" && d == desc
				}
				provider := func(owner string) (string, map[string]string, bool) {
					switch owner {
					case "probe/Child":
						return "<V:Ljava/lang/Object;>Lprobe/Parent<TV;>;", map[string]string{class_context.MethodDescKey("get", desc): ""}, true
					case "probe/Parent":
						return "<X:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("get", desc): "(" + input + ")TX;"}, true
					}
					return "", nil, false
				}
				_, result, _ := ResolveInstantiatedSignatureExact(ctx, provider, "probe.Child", []JavaType{NewJavaClass("T")}, "get", desc, len(input))
				if (result != nil) != (bound == "java/lang/Object") {
					t.Fatalf("new result payload check accepted: %v", result)
				}
			})
		}
	}
}
