package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestUniqueStaticCallDoesNotNameInaccessibleAncestor(t *testing.T) {
	ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
		switch name {
		case "library/Factory":
			return callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "read", Desc: "(Llibrary/Hidden;)Ljava/lang/Object;", Public: true, Static: true}}}, true
		case "library/Concrete":
			return callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"library/Hidden"}}, true
		case "library/Hidden":
			return callbinding.Class{Name: name, MembersComplete: true, ParentsComplete: true}, true
		}
		return callbinding.Class{}, false
	}}
	id := utils.NewRootVariableId()
	id.SetName("payload")
	mt, err := types.ParseMethodDescriptor("(Llibrary/Hidden;)Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	call := &FunctionCallExpression{ClassName: "library.Factory", Object: NewJavaClassValue(types.NewJavaClass("library.Factory")), FunctionName: "read", Descriptor: "(Llibrary/Hidden;)Ljava/lang/Object;", IsStatic: true, Kind: InvokeStatic, FuncType: mt.FunctionType(), Arguments: []JavaValue{NewJavaRef(id, nil, types.NewJavaClass("library.Concrete"))}}
	if got := call.String(ctx); got != "Factory.read(payload)" {
		t.Fatalf("implicit reference widening must not introduce an inaccessible source type: %s", got)
	}
	if ctx.OverloadFamilyUnproven {
		t.Fatal("complete unique static binding classified unknown")
	}
	call.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
	if got := call.String(ctx); got != "Factory.read(null)" {
		t.Fatalf("unique null widening named inaccessible ancestor: %s", got)
	}
}

func TestStaticNullCallRespectsScopedCastPolicy(t *testing.T) {
	t.Setenv("JDEC_NULL_ARG_CAST_OFF", "1")
	for _, variant := range []string{"closed default snapshot", "explicit disabled snapshot"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{Env: func(key string) string {
				if variant == "explicit disabled snapshot" && key == "JDEC_NULL_ARG_CAST_OFF" {
					return "1"
				}
				return ""
			}, InvocationMetadata: func(name string) (callbinding.Class, bool) {
				if name != "policy/Factory" {
					return callbinding.Class{}, false
				}
				return callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "read", Desc: "(Ljava/lang/Object;)Ljava/lang/String;", Public: true, Static: true}, {Name: "read", Desc: "(Ljava/lang/String;)Ljava/lang/String;", Public: true, Static: true}}}, true
			}}
			desc := "(Ljava/lang/Object;)Ljava/lang/String;"
			mt, e := types.ParseMethodDescriptor(desc)
			if e != nil {
				t.Fatal(e)
			}
			call := &FunctionCallExpression{ClassName: "policy.Factory", Object: NewJavaClassValue(types.NewJavaClass("policy.Factory")), FunctionName: "read", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, FuncType: mt.FunctionType(), Arguments: []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}}
			want := "Factory.read((Object)(null))"
			if variant == "explicit disabled snapshot" {
				want = "Factory.read(null)"
			}
			if got := call.String(ctx); got != want {
				t.Fatalf("request-scoped null descriptor pin %q want %q", got, want)
			}
		})
	}
}

func TestStaticImplicitWideningRequiresCompleteExactSelection(t *testing.T) {
	for _, variant := range []string{"unique", "competing", "missing parent", "incomplete members", "incomplete parents", "missing owner", "wrong owner identity", "nonpublic owner", "nonpublic method", "generic", "varargs", "bridge", "instance target", "virtual call", "inconsistent static flag", "special call", "covariant hiding", "different descriptor", "hierarchy cycle"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Llibrary/Hidden;)Ljava/lang/Object;"
			meta := callbinding.Class{Name: "library/Factory", Public: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "read", Desc: desc, Public: true, Static: true}}}
			parent := callbinding.Class{Name: "library/Parent", Public: true, MembersComplete: true, ParentsComplete: true}
			call := &FunctionCallExpression{ClassName: "library.Factory", FunctionName: "read", Descriptor: desc, IsStatic: true, Kind: InvokeStatic}
			switch variant {
			case "competing":
				meta.Methods = append(meta.Methods, callbinding.Method{Name: "read", Desc: "(Llibrary/Concrete;)Ljava/lang/Object;", Public: true, Static: true})
			case "missing parent":
				meta.Parents = []string{"library/Missing"}
			case "incomplete members":
				meta.MembersComplete = false
			case "incomplete parents":
				meta.ParentsComplete = false
			case "wrong owner identity":
				meta.Name = "library/Other"
			case "nonpublic owner":
				meta.Public = false
			case "nonpublic method":
				meta.Methods[0].Public = false
			case "generic":
				meta.Methods[0].Generic = true
			case "varargs":
				meta.Methods[0].Varargs = true
			case "bridge":
				meta.Methods[0].Bridge = true
			case "instance target":
				meta.Methods[0].Static = false
			case "virtual call":
				call.IsStatic = false
				call.Kind = InvokeVirtual
			case "inconsistent static flag":
				call.IsStatic = false
			case "special call":
				call.IsSpecialInvoke = true
			case "covariant hiding":
				meta.Parents = []string{parent.Name}
				parent.Methods = []callbinding.Method{{Name: "read", Desc: "(Llibrary/Hidden;)Ljava/lang/String;", Public: true, Static: true}}
			case "different descriptor":
				meta.Methods[0].Desc = "(Llibrary/Hidden;)Ljava/lang/String;"
			case "hierarchy cycle":
				meta.Parents = []string{meta.Name}
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
				switch name {
				case "library/Factory":
					return meta, variant != "missing owner"
				case parent.Name:
					return parent, true
				case "library/Concrete":
					return callbinding.Class{Name: name, Public: true, Parents: []string{"library/Hidden"}, MembersComplete: true, ParentsComplete: true}, true
				case "library/Hidden":
					return callbinding.Class{Name: name, MembersComplete: true, ParentsComplete: true}, true
				}
				return callbinding.Class{}, false
			}}
			want := variant == "unique"
			if got := call.staticCallHasUniqueErasedBinding(ctx); got != want {
				t.Fatalf("static source selection %v want %v", got, want)
			}
			// Virtual calls have their existing separate proof. Every other
			// refusal must also survive the ordinary argument renderer.
			if variant != "virtual call" {
				if got := call.unprovenWideningArgCast(types.NewJavaClass("library.Concrete"), types.NewJavaClass("library.Hidden"), ctx); got != want {
					t.Fatalf("renderer widening %v want %v", got, want)
				}
			}
		})
	}
}
