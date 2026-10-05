package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestGenericArraySpreadRequiresExactVarargsDeclaration(t *testing.T) {
	for _, scenario := range []string{"varargs", "narrower declaration", "poly element", "null element", "array element", "known lexical bound", "narrower elements", "narrower allocation", "missing lexical bound", "dynamic call", "ordinary array", "same arity wrong flags", "arity only signature", "wrong descriptor signature", "wrong static kind", "bridge", "missing owner", "incomplete members", "missing parent", "inherited", "different arity overload", "competing overload", "switch off", "nil context"} {
		t.Run(scenario, func(t *testing.T) {
			const owner = "example/Owner"
			const desc = "([Ljava/lang/Object;)Ljava/lang/Object;"
			const sig = "<T:Ljava/lang/Object;>([TT;)TT;"
			meta := map[string]callbinding.Class{
				owner: {Name: owner, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "pick", Desc: desc, Varargs: true, Static: true, Generic: true}}},
			}
			signatures := map[string]map[string]string{owner: {class_context.MethodDescKey("pick", desc): sig, class_context.MethodSigKey("pick", 1): sig}}
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { v, ok := meta[n]; return v, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) { v, ok := signatures[n]; return "", v, ok }}
			array := NewNewExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
			array.Initializer = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object")), NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))}
			call := &FunctionCallExpression{ClassName: owner, FunctionName: "pick", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, Arguments: []JavaValue{array}}
			cls := meta[owner]
			switch scenario {
			case "narrower declaration":
				array.Initializer[0].(*JavaRef).WebDeclType = types.NewJavaClass("java.lang.String")
			case "poly element":
				array.Initializer[0] = &FunctionCallExpression{FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("java.lang.Object")}}
			case "null element":
				array.Initializer[0] = JavaNull
			case "array element":
				array.Initializer[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
			case "known lexical bound":
				ctx.TypeParams = []string{"N"}
				ctx.ClassSig = "<N:Ljava/lang/Object;>Ljava/lang/Object;"
				array.Initializer[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("N"))
			case "narrower elements":
				array.Initializer = []JavaValue{NewJavaLiteral("a", types.NewJavaClass("java.lang.String")), NewJavaLiteral("b", types.NewJavaClass("java.lang.String"))}
			case "narrower allocation":
				array.JavaType = types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
				array.Initializer = []JavaValue{NewJavaLiteral("a", types.NewJavaClass("java.lang.String")), NewJavaLiteral("b", types.NewJavaClass("java.lang.String"))}
			case "missing lexical bound":
				ctx.TypeParams = []string{"N"}
				array.Initializer[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("N"))
			case "dynamic call":
				call.Kind = InvokeDynamic
			case "ordinary array":
				cls.Methods[0].Varargs = false
			case "same arity wrong flags":
				cls.Methods[0].Varargs = false
				cls.Methods = append(cls.Methods, callbinding.Method{Name: "pick", Desc: "([Ljava/lang/String;)Ljava/lang/Object;", Varargs: true, Static: true})
			case "arity only signature":
				delete(signatures[owner], class_context.MethodDescKey("pick", desc))
			case "wrong descriptor signature":
				signatures[owner][class_context.MethodDescKey("pick", desc)] = "<T:Ljava/lang/String;>([TT;)TT;"
			case "wrong static kind":
				cls.Methods[0].Static = false
			case "bridge":
				cls.Methods[0].Bridge = true
			case "missing owner":
				delete(meta, owner)
			case "incomplete members":
				cls.MembersComplete = false
			case "missing parent":
				cls.Parents = []string{"example/Missing"}
			case "inherited":
				cls.Parents = []string{"example/Base"}
				meta["example/Base"] = callbinding.Class{Name: "example/Base", MembersComplete: true, ParentsComplete: true, Methods: cls.Methods}
				cls.Methods = nil
				signatures["example/Base"] = signatures[owner]
				signatures[owner] = map[string]string{}
			case "different arity overload":
				cls.Methods = append(cls.Methods, callbinding.Method{Name: "pick", Desc: "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", Static: true})
			case "competing overload":
				cls.Methods = append(cls.Methods, callbinding.Method{Name: "pick", Desc: "(Ljava/lang/Object;)Ljava/lang/Object;", Static: true})
			case "switch off":
				ctx.Env = func(k string) string {
					if k == "JDEC_VARARGS_SPREAD_OFF" {
						return "1"
					}
					return ""
				}
			case "nil context":
				ctx = nil
			}
			if scenario != "missing owner" {
				meta[owner] = cls
			}
			got, fixed, ok := call.varargsTypeVarSpread(ctx)
			want := scenario == "varargs" || scenario == "inherited" || scenario == "known lexical bound"
			if ok != want {
				t.Fatalf("spread=%v want=%v", ok, want)
			}
			if want && (fixed != 0 || len(got) != 2 || got[0] != array.Initializer[0] || got[1] != array.Initializer[1]) {
				t.Fatal("changed original operand identity/order")
			}
			if call.Arguments[0] != array || array.JavaType.ArrayDim() != 1 || len(array.Initializer) != 2 {
				t.Fatal("proof mutated shared array/call")
			}
		})
	}
}
