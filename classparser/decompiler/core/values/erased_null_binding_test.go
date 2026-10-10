package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestErasedNullBindingKeepsDescriptorAndEvaluationIdentity(t *testing.T) {
	for _, sig := range []string{"Lexample/Sink<TT;>;", "Lexample/Sink<-TT;>;", "Lexample/Sink<+TT;>;", "Lexample/Sink<*>;"} {
		ctx := &class_context.ClassContext{TypeParams: []string{"T"}}
		receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.ParseSignature(sig))
		receiver.Id.SetName("sink")
		arg := NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
		call := &FunctionCallExpression{Object: receiver, ClassName: "example.Sink", FunctionName: "consume", Descriptor: "(Ljava/lang/Object;)V", Kind: InvokeInterface, OriginPC: 17, Arguments: []JavaValue{arg}}
		planned, ok := call.planErasedNullBinding(ctx)
		if !ok || planned.Witness() != call.Witness() || !planned.bindingPlanned {
			t.Fatalf("%s: erased invoke witness was not retained", sig)
		}
		recvCast, argCast := planned.Object.(*CastExpression), planned.Arguments[0].(*CastExpression)
		if recvCast.Value != receiver || argCast.Value != arg || !recvCast.Binding || !argCast.Binding {
			t.Fatal("binding copied or replaced an evaluated value")
		}
		if call.Object != receiver || call.Arguments[0] != arg {
			t.Fatal("planning mutated the shared original invocation")
		}
		if got := planned.String(ctx); got != "((example.Sink)(sink)).consume((Object)(null))" {
			t.Fatalf("descriptor rendering=%q", got)
		}
	}
}

func TestErasedNullBindingPreservesProvenUniqueGenericFormal(t *testing.T) {
	const desc = "(Ljava/lang/Object;)V"
	ctx := &class_context.ClassContext{
		TypeParams: []string{"T"},
		SiblingClassSig: func(name string) (string, map[string]string, bool) {
			return "<E:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("consume", desc): "(TE;)V"}, name == "example/Sink"
		},
		InvocationMetadata: func(name string) (callbinding.Class, bool) {
			return callbinding.Class{Name: name, Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "consume", Desc: desc, Public: true, Generic: true}}}, name == "example/Sink"
		},
	}
	receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.ParseSignature("Lexample/Sink<TT;>;"))
	call := &FunctionCallExpression{Object: receiver, ClassName: "example.Sink", FunctionName: "consume", Descriptor: desc, Kind: InvokeInterface, Arguments: []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}}
	if _, ok := call.planErasedNullBinding(ctx); ok {
		t.Fatal("an already proven generic formal was needlessly erased")
	}
	metadata := ctx.InvocationMetadata
	ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
		c, ok := metadata(name)
		c.Methods = append(c.Methods, callbinding.Method{Name: "consume", Desc: "(Ljava/lang/String;)V", Public: true})
		return c, ok
	}
	if _, ok := call.planErasedNullBinding(ctx); !ok {
		t.Fatal("a competing String overload stole the Object descriptor")
	}
}

func TestErasedNullBindingRejectsUnprovenCalls(t *testing.T) {
	for _, scenario := range []string{"static", "special", "dynamic", "constructor", "other owner", "raw receiver", "missing receiver", "nonvoid", "array formal", "string formal", "missing descriptor", "extra argument", "string spelling null", "nonnull", "effectful argument", "cast argument", "missing context"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.ParseSignature("Lexample/Sink<-TT;>;"))
			arg := NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			call := &FunctionCallExpression{Object: receiver, ClassName: "example.Sink", FunctionName: "consume", Descriptor: "(Ljava/lang/Object;)V", Kind: InvokeVirtual, Arguments: []JavaValue{arg}}
			switch scenario {
			case "static":
				call.IsStatic = true
			case "special":
				call.IsSpecialInvoke = true
			case "dynamic":
				call.Kind = InvokeDynamic
			case "constructor":
				call.FunctionName = "<init>"
			case "other owner":
				call.ClassName = "example.Parent"
			case "raw receiver":
				call.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Sink"))
			case "missing receiver":
				call.Object = nil
			case "nonvoid":
				call.Descriptor = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "array formal":
				call.Descriptor = "([Ljava/lang/Object;)V"
			case "string formal":
				call.Descriptor = "(Ljava/lang/String;)V"
			case "missing descriptor":
				call.Descriptor = ""
			case "extra argument":
				call.Arguments = append(call.Arguments, arg)
			case "string spelling null":
				call.Arguments[0] = NewJavaLiteral("null", types.NewJavaClass("java.lang.String"))
			case "nonnull":
				call.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "effectful argument":
				call.Arguments[0] = &FunctionCallExpression{FunctionName: "sideEffect"}
			case "cast argument":
				call.Arguments[0] = &CastExpression{Value: arg, TargetType: types.NewJavaClass("java.lang.String")}
			case "missing context":
				ctx = nil
			}
			if _, ok := call.planErasedNullBinding(ctx); ok {
				t.Fatal("unproven raw-receiver rewrite was accepted")
			}
		})
	}
}
