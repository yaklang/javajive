package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestCovariantOverloadResultRequiresDeclarationEvidence(t *testing.T) {
	for _, scenario := range []string{"proved", "missing unrelated ancestor", "unknown receiver", "incomplete receiver", "unique consumer", "generic consumer", "varargs consumer", "generic override", "bridge only", "wrong name", "argumentful producer", "non-covariant result", "special producer", "different descriptor result"} {
		t.Run(scenario, func(t *testing.T) {
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Dual"))
			child := &FunctionCallExpression{Object: receiver, ClassName: "p.Left", FunctionName: "copy", Descriptor: "()Lp/Left;", Kind: InvokeInterface, OriginPC: 11, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("p.Left")}}
			call := &FunctionCallExpression{ClassName: "p.Calls", FunctionName: "consume", Descriptor: "(Lp/Left;)I", IsStatic: true, Kind: InvokeStatic, OriginPC: 13, Arguments: []JavaValue{child}}
			classes := map[string]callbinding.Class{
				"p/Calls": {Name: "p/Calls", Public: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "consume", Desc: call.Descriptor, Static: true, Public: true}, {Name: "consume", Desc: "(Lp/Right;)I", Static: true, Public: true}}},
				"p/Dual":  {Name: "p/Dual", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"p/Left", "p/Right"}, Methods: []callbinding.Method{{Name: "copy", Desc: "()Lp/Dual;", Public: true}}},
				"p/Left":  {Name: "p/Left", Public: true, MembersComplete: true, ParentsComplete: true},
				"p/Right": {Name: "p/Right", Public: true, MembersComplete: true, ParentsComplete: true},
			}
			dual, calls := classes["p/Dual"], classes["p/Calls"]
			switch scenario {
			case "missing unrelated ancestor":
				dual.Parents = append(dual.Parents, "p/Missing")
			case "unknown receiver":
				delete(classes, "p/Dual")
			case "incomplete receiver":
				dual.MembersComplete = false
			case "unique consumer":
				calls.Methods = calls.Methods[:1]
			case "generic consumer":
				calls.Methods[0].Generic = true
			case "varargs consumer":
				calls.Methods[0].Varargs = true
			case "generic override":
				dual.Methods[0].Generic = true
			case "bridge only":
				dual.Methods[0].Bridge = true
			case "wrong name":
				dual.Methods[0].Name = "other"
			case "argumentful producer":
				child.Descriptor = "(I)Lp/Left;"
				dual.Methods[0].Desc = "(I)Lp/Dual;"
			case "non-covariant result":
				dual.Methods[0].Desc = "()Lp/Unrelated;"
			case "special producer":
				child.IsSpecialInvoke = true
			case "different descriptor result":
				child.Descriptor = "()Lp/Right;"
			}
			if scenario != "unknown receiver" {
				classes["p/Dual"] = dual
			}
			classes["p/Calls"] = calls
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { v, ok := classes[n]; return v, ok }}
			want := ""
			if scenario == "proved" || scenario == "missing unrelated ancestor" {
				want = "Left"
			}
			if got := call.covariantOverloadResultCast(0, ctx); got != want {
				t.Fatalf("cast=%q want %q", got, want)
			}
			if call.Arguments[0] != child || child.Object != receiver || child.OriginPC != 11 {
				t.Fatal("modified evaluation identity/witness")
			}
		})
	}
}
