package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestFunctionalTargetThroughGenericMethodUsesOriginalArgumentWitnesses(t *testing.T) {
	for _, kind := range []string{"typed", "erased", "erased first", "static", "static erased first", "renamed caller", "method shadows class", "same bounded erasure", "new payload check", "missing caller bound", "dependent caller bound", "concrete payload", "raw invariant", "conflicting invariant", "missing declaration", "other descriptor", "contradictory descriptor", "foreign result", "renderer disabled", "budget", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			desc := "(Lwitness/Box;Ljava/lang/Object;)Ljava/util/Optional;"
			sig := "<T:Ljava/lang/Object;>(Lwitness/Box<TT;>;TT;)Ljava/util/Optional<TT;>;"
			caller := "T"
			ctx := &class_context.ClassContext{ClassName: "witness.Caller", ClassSig: "Ljava/lang/Object;", CurrentMethodSig: "<T:Ljava/lang/Object;>(Lwitness/Box<TT;>;Ljava/lang/Object;)TT;", TypeParams: []string{"T"}}
			var word types.JavaType = types.NewJavaClass("java.lang.Object")
			switch kind {
			case "typed":
				word = types.NewJavaClass("T")
			case "erased first", "static erased first":
				desc = "(Ljava/lang/Object;Lwitness/Box;)Ljava/util/Optional;"
				sig = "<T:Ljava/lang/Object;>(TT;Lwitness/Box<TT;>;)Ljava/util/Optional<TT;>;"
			case "renamed caller":
				caller = "S"
				ctx.TypeParams = []string{"S"}
				ctx.CurrentMethodSig = "<S:Ljava/lang/Object;>(Lwitness/Box<TS;>;Ljava/lang/Object;)TS;"
			case "method shadows class":
				ctx.ClassSig = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				ctx.ClassTypeParams = []string{"T"}
			case "same bounded erasure":
				ctx.CurrentMethodSig = "<T:Ljava/lang/Number;>(Lwitness/Box<TT;>;Ljava/lang/Number;)TT;"
				desc = "(Lwitness/Box;Ljava/lang/Number;)Ljava/util/Optional;"
				sig = "<T:Ljava/lang/Number;>(Lwitness/Box<TT;>;TT;)Ljava/util/Optional<TT;>;"
				word = types.NewJavaClass("java.lang.Number")
			case "new payload check":
				ctx.CurrentMethodSig = "<T:Ljava/lang/Number;>(Lwitness/Box<TT;>;Ljava/lang/Object;)TT;"
			case "missing caller bound":
				ctx.CurrentMethodSig = "(Lwitness/Box;Ljava/lang/Object;)Ljava/lang/Object;"
			case "dependent caller bound":
				ctx.CurrentMethodSig = "<U:Ljava/lang/Number;T:TU;>(Lwitness/Box<TT;>;Ljava/lang/Object;)TT;"
				ctx.TypeParams = []string{"T", "U"}
			case "concrete payload":
				caller = "java.lang.String"
			case "conflicting invariant":
				desc = "(Lwitness/Box;Lwitness/Box;Ljava/lang/Object;)Ljava/util/Optional;"
				sig = "<T:Ljava/lang/Object;>(Lwitness/Box<TT;>;Lwitness/Box<TT;>;TT;)Ljava/util/Optional<TT;>;"
				ctx.TypeParams = []string{"T", "S"}
				ctx.CurrentMethodSig = "<T:Ljava/lang/Object;S:Ljava/lang/Object;>()V"
			case "contradictory descriptor":
				sig = "<T:Ljava/lang/Number;>(Lwitness/Box<TT;>;TT;)Ljava/util/Optional<TT;>;"
			case "foreign result":
				sig = "<T:Ljava/lang/Object;>(Lwitness/Box<TT;>;TT;)Ljava/util/Optional<TX;>;"
			case "renderer disabled":
				t.Setenv("JDEC_GENERIC_METHOD_WITNESS_OFF", "1")
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				if n == "java/util/Optional" {
					return "<V:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("orElseGet", "(Ljava/util/function/Supplier;)Ljava/lang/Object;"): "(Ljava/util/function/Supplier<+TV;>;)TV;"}, true
				}
				if n != "witness/Maker" || kind == "missing declaration" {
					return "", nil, false
				}
				key := desc
				if kind == "other descriptor" {
					key = "(Lwitness/Box;Ljava/lang/Number;)Ljava/util/Optional;"
				}
				return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("pick", key): sig}, true
			}
			mt, e := types.ParseMethodDescriptor(desc)
			if e != nil {
				t.Fatal(e)
			}
			pick := NewFunctionCallExpression(NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("witness.Maker")), NewJavaClassMember("witness.Maker", "pick", desc, mt), mt.FunctionType())
			pick.Descriptor = desc
			box := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("witness.Box", []types.JavaType{types.NewJavaClass(caller)}))
			arg := NewJavaRef(utils.NewRootVariableId(), nil, word)
			pick.Arguments = []JavaValue{box, arg}
			if kind == "erased first" || kind == "static erased first" {
				pick.Arguments = []JavaValue{arg, box}
			}
			if kind == "static" || kind == "static erased first" {
				pick.IsStatic = true
				pick.Kind = InvokeStatic
			}
			if kind == "raw invariant" {
				pick.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("witness.Box"))
			}
			if kind == "conflicting invariant" {
				pick.Arguments = []JavaValue{box, NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("witness.Box", []types.JavaType{types.NewJavaClass("S")})), arg}
			}
			outerDesc := "(Ljava/util/function/Supplier;)Ljava/lang/Object;"
			ot, e := types.ParseMethodDescriptor(outerDesc)
			if e != nil {
				t.Fatal(e)
			}
			outer := NewFunctionCallExpression(pick, NewJavaClassMember("java.util.Optional", "orElseGet", outerDesc, ot), ot.FunctionType())
			outer.Descriptor = outerDesc
			outer.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.Supplier"))}
			result := outer.FunctionalTargetParamType(0, ctx)
			positive := kind == "typed" || kind == "erased" || kind == "erased first" || kind == "static" || kind == "static erased first" || kind == "renamed caller" || kind == "method shadows class" || kind == "same bounded erasure"
			if positive {
				if result == nil || result.String(ctx) != "Supplier<? extends "+caller+">" {
					t.Fatalf("target=%v", result)
				}
			} else if result != nil {
				t.Fatalf("unproved source target=%s", result.String(ctx))
			}
		})
	}
}
