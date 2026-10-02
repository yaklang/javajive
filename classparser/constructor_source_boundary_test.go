package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestConstructorCarrierRejectsUnprovedValueGraphs(t *testing.T) {
	for _, kind := range []string{"parameter", "missing value", "typed nil", "unknown parameter", "this", "custom parameter", "missing PC", "missing descriptor", "dynamic", "special", "allocation", "cycle", "budget", "duplicate PC", "instance parameter", "instance this"} {
		t.Run(kind, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Value"))
			ref.IsParam = true
			allowed := map[*values.JavaRef]bool{ref: true}
			call := &values.FunctionCallExpression{IsStatic: true, ClassName: "p.Actions", FunctionName: "check", Descriptor: "(Ljava/lang/Object;)V", Kind: values.InvokeStatic, OriginPC: 3, HasOriginPC: true, Arguments: []values.JavaValue{ref}}
			var value values.JavaValue = call
			calls := map[int]*values.FunctionCallExpression{}
			remaining := 1024
			switch kind {
			case "missing value":
				call.Arguments[0] = nil
			case "typed nil":
				call.Arguments[0] = (*values.JavaArrayMember)(nil)
			case "unknown parameter":
				delete(allowed, ref)
			case "this":
				ref.IsThis = true
			case "custom parameter":
				ref.CustomValue = &values.CustomValue{}
			case "missing PC":
				call.HasOriginPC = false
			case "missing descriptor":
				call.Descriptor = ""
			case "dynamic":
				call.Kind = values.InvokeDynamic
			case "special":
				call.Kind = values.InvokeSpecial
			case "allocation":
				call.FunctionName = "<init>"
			case "cycle":
				c := &values.CustomValue{Flag: "lambda", CapturesKnown: true}
				c.Captures = []values.JavaValue{c}
				call.Arguments[0] = c
			case "budget":
				remaining = 0
			case "duplicate PC":
				copy := *call
				calls[3] = &copy
			case "instance parameter":
				call.IsStatic = false
				call.Kind = values.InvokeVirtual
				call.Object = ref
			case "instance this":
				call.IsStatic = false
				call.Kind = values.InvokeVirtual
				call.Object = ref
				ref.IsThis = true
			}
			active := map[values.JavaValue]bool{}
			got := constructorBoundaryValue(value, allowed, active, calls, &remaining)
			want := kind == "parameter" || kind == "instance parameter"
			if got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
			if len(active) != 0 {
				t.Fatal("proof retained active visitor state")
			}
		})
	}
}

func TestAdversarialConstructorCarrierKeepsGenericSAMTarget(t *testing.T) {
	roundTripGenericFlowUnits(t, "ConstructorSAMTargetDriver", `import java.util.function.Consumer;
class ConstructorSAMActions {static String trace="";void append(String value){trace+="S"+value;}void append(Object value){trace+="O"+value;}}
class ConstructorSAMOwner {final Consumer<String> sink;ConstructorSAMOwner(Consumer<String> value){sink=value;}ConstructorSAMOwner(ConstructorSAMActions actions){this(actions::append);}}
public class ConstructorSAMTargetDriver {public static void main(String[]args){try{new ConstructorSAMOwner((ConstructorSAMActions)null);throw new AssertionError("missing null check");}catch(NullPointerException expected){System.out.println("null");}ConstructorSAMOwner owner=new ConstructorSAMOwner(new ConstructorSAMActions());owner.sink.accept("x");if(!ConstructorSAMActions.trace.equals("Sx"))throw new AssertionError("overload");try{((Consumer)owner.sink).accept(new Object());throw new AssertionError("lost SAM cast");}catch(ClassCastException expected){System.out.println(ConstructorSAMActions.trace);}}}`, nil, []string{"ConstructorSAMOwner"}, Precision, Compatibility, "legacy")
}

func TestConstructorPrefixOriginalInvokeIdentity(t *testing.T) {
	member := values.NewJavaClassMember("p/Actions", "check", "(Ljava/lang/Object;)V", types.NewJavaClass("p.Actions"))
	get := func(int) values.JavaValue { return member }
	decoder := core.NewDecompiler([]byte{0, 0, 0, 184, 0, 1, 177}, get)
	if err := decoder.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"exact", "no PC", "negative PC", "overflow PC", "NOP PC", "owner", "name", "descriptor", "kind", "static flag"} {
		t.Run(kind, func(t *testing.T) {
			call := &values.FunctionCallExpression{ClassName: "p.Actions", FunctionName: "check", Descriptor: "(Ljava/lang/Object;)V", Kind: values.InvokeStatic, IsStatic: true, OriginPC: 3, HasOriginPC: true}
			switch kind {
			case "no PC":
				call.HasOriginPC = false
			case "negative PC":
				call.OriginPC = -1
			case "overflow PC":
				call.OriginPC = 65539
			case "NOP PC":
				call.OriginPC = 0
			case "owner":
				call.ClassName = "p.Other"
			case "name":
				call.FunctionName = "other"
			case "descriptor":
				call.Descriptor = "()V"
			case "kind":
				call.Kind = values.InvokeVirtual
			case "static flag":
				call.IsStatic = false
			}
			if got := constructorOriginalInvoke(decoder, call, get); got != (kind == "exact") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestConstructorStaticCarrierTypeRequiresClosedBindings(t *testing.T) {
	ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
		if name == "p/Box" || name == "java/lang/String" {
			return callbinding.Class{Name: name, Public: true}, true
		}
		return callbinding.Class{}, false
	}}
	stringType := types.NewJavaClass("java.lang.String")
	if !constructorStaticSourceType(types.NewParameterizedType("p.Box", []types.JavaType{stringType}), ctx) {
		t.Fatal("concrete signature refused")
	}
	if constructorStaticSourceType(types.NewParameterizedType("p.Box", []types.JavaType{types.NewJavaClass("T")}), ctx) {
		t.Fatal("unbound formal entered static scope")
	}
	if constructorStaticSourceType(types.NewJavaClass("p.Missing"), ctx) {
		t.Fatal("unknown declaration guessed accessible")
	}
	var deep types.JavaType = stringType
	for i := 0; i < 129; i++ {
		deep = types.NewParameterizedType("p.Box", []types.JavaType{deep})
	}
	if constructorStaticSourceType(deep, ctx) {
		t.Fatal("type proof exceeded budget")
	}
	cyclic := types.NewParameterizedType("p.Box", nil)
	raw, _ := types.AsParameterizedType(cyclic)
	raw.TypeArgs = []types.JavaType{cyclic}
	if constructorStaticSourceType(cyclic, ctx) {
		t.Fatal("cyclic type accepted")
	}
	ctx.InvocationMetadata = func(string) (callbinding.Class, bool) { return callbinding.Class{Name: "p/Other", Public: true}, true }
	if constructorStaticSourceType(stringType, ctx) {
		t.Fatal("wrong declaration identity accepted")
	}
}
