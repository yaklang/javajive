package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestFixedFactoryAssignmentRequiresClosedNonPolyInvocationTree(t *testing.T) {
	for _, variant := range []string{"original", "missing metadata", "unknown signature", "no origin", "incomplete family", "overload", "bridge", "instance invocation", "dynamic", "wrong arity", "generic throws", "bare result", "lambda", "object allocation", "cycle", "different erasure", "raw destination"} {
		t.Run(variant, func(t *testing.T) {
			descriptor := "([Ljava/lang/Object;)Lexample/Bag;"
			signature := "<E:Ljava/lang/Object;>([TE;)Lexample/Bag<TE;>;"
			methods := []callbinding.Method{{Name: "pack", Desc: descriptor, Static: true, Public: true, Generic: true, Varargs: true}}
			meta := map[string]callbinding.Class{
				"example/Maker":    {Name: "example/Maker", Public: true, MembersComplete: true, ParentsComplete: true, Methods: methods},
				"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true},
				"example/Bag":      {Name: "example/Bag", Public: true, MembersComplete: true, ParentsComplete: true},
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) { c, ok := meta[name]; return c, ok }, SiblingClassSig: func(name string) (string, map[string]string, bool) {
				return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("pack", descriptor): signature}, name == "example/Maker"
			}}
			array := NewNewExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
			array.Initializer = []JavaValue{JavaNull}
			ft, _ := types.ParseMethodDescriptor(descriptor)
			call := &FunctionCallExpression{ClassName: "example.Maker", FunctionName: "pack", Descriptor: descriptor, IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 12, FuncType: ft.FunctionType(), Arguments: []JavaValue{array}}
			var target types.JavaType = types.NewParameterizedType("example.Bag", []types.JavaType{types.NewJavaClass("java.lang.String")})
			owner := meta["example/Maker"]
			switch variant {
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "unknown signature":
				signature = ""
			case "no origin":
				call.HasOriginPC = false
			case "incomplete family":
				owner.MembersComplete = false
			case "overload":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "pack", Desc: "(Ljava/lang/Object;)Lexample/Bag;", Static: true, Public: true})
			case "bridge":
				owner.Methods[0].Bridge = true
			case "instance invocation":
				call.IsStatic = false
				call.Kind = InvokeVirtual
			case "dynamic":
				call.Kind = InvokeDynamic
			case "wrong arity":
				call.Arguments = nil
			case "generic throws":
				signature += "^TE;"
			case "bare result":
				signature = "<E:Ljava/lang/Object;>([TE;)TE;"
			case "lambda":
				array.Initializer = []JavaValue{&CustomValue{Flag: "lambda"}}
			case "object allocation":
				array.Initializer = []JavaValue{NewNewExpression(types.NewJavaClass("java.lang.Object"))}
			case "cycle":
				array.Initializer = []JavaValue{array}
			case "different erasure":
				target = types.NewParameterizedType("example.Other", []types.JavaType{types.NewJavaClass("java.lang.String")})
			case "raw destination":
				target = types.NewJavaClass("example.Bag")
			}
			meta["example/Maker"] = owner
			before := call.Witness()
			result, proved := FixedFactoryAssignmentView(call, target, ctx)
			if proved != (variant == "original") {
				t.Fatalf("proved=%v", proved)
			}
			if proved {
				cast, ok := result.(*CastExpression)
				if !ok || cast.Value != call || bindingType(cast.TargetType) != "Lexample/Bag;" || call.Witness() != before || call.Arguments[0] != array {
					t.Fatal("changed invocation, operand identity or runtime result type")
				}
			} else if result != call {
				t.Fatal("failed proof altered expression")
			}
		})
	}
}
