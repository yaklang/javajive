package values

import (
	"testing"

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
		if got := planned.String(ctx); got != "((Sink)(sink)).consume((Object)(null))" {
			t.Fatalf("descriptor rendering=%q", got)
		}
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
