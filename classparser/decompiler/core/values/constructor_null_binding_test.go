package values

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestConstructorBareNullRequiresUniqueCompleteOriginalBinding(t *testing.T) {
	for _, variant := range []string{"reference", "array", "this", "different arity", "missing owner", "wrong owner", "incomplete", "missing target", "duplicate target", "same arity", "varargs rival", "selected varargs", "invalid rival", "static rival", "bridge rival", "class generic", "generic superclass", "class formal parameter", "method generic", "signature without flag", "no origin", "later null", "synthetic null", "copied null", "original cast", "allocation", "other owner", "ordinary method", "wrong invoke", "primitive", "wrong arity"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Ljava/lang/String;)V"
			if variant == "array" {
				desc = "([I)V"
			}
			table := callbinding.Class{Name: "proof/Base", MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc}}}
			ctx := &class_context.ClassContext{ClassName: "proof.Child", SupperClassName: "proof.Base", FunctionName: "<init>"}
			ctx.InvocationMetadata = func(string) (callbinding.Class, bool) { return table, variant != "missing owner" }
			receiver := NewJavaRef(nil, nil, types.NewJavaClass("proof.Child"))
			receiver.IsThis = true
			literal := NewOriginalNullLiteral(5)
			call := &FunctionCallExpression{ClassName: "proof.Base", FunctionName: "<init>", Descriptor: desc, Object: receiver, Arguments: []JavaValue{literal}, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 6, HasOriginPC: true}
			want := false
			switch variant {
			case "reference", "array":
				want = true
			case "this":
				ctx.ClassName = "proof.Base"
				want = true
			case "different arity":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "()V"})
				want = true
			case "wrong owner":
				table.Name = "proof/Other"
			case "incomplete":
				table.MembersComplete = false
			case "missing target":
				table.Methods = nil
			case "duplicate target":
				table.Methods = append(table.Methods, table.Methods[0])
			case "same arity":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "(Ljava/lang/Integer;)V"})
			case "varargs rival":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "(I[Ljava/lang/Object;)V", Varargs: true})
			case "selected varargs":
				table.Methods[0].Varargs = true
			case "invalid rival":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "()I"})
			case "static rival":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "()V", Static: true})
			case "bridge rival":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "()V", Bridge: true})
			case "class generic":
				table.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				want = true
			case "generic superclass":
				table.Signature = "Ljava/util/ArrayList<Ljava/lang/String;>;"
				want = true
			case "class formal parameter":
				want = true
				table.Signature = "<T:Ljava/lang/String;>Ljava/lang/Object;"
				table.Methods[0].Signature = "(TT;)V"
				table.Methods[0].Generic = true
			case "method generic":
				table.Methods[0].Generic = true
			case "signature without flag":
				table.Methods[0].Signature = "<T:Ljava/lang/String;>(TT;)V"
			case "no origin":
				call.HasOriginPC = false
			case "later null":
				call.OriginPC = 4
			case "synthetic null":
				call.Arguments[0] = NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "copied null":
				copied := *literal
				call.Arguments[0] = &copied
			case "original cast":
				call.Arguments[0] = &CastExpression{Value: literal, TargetType: types.NewJavaClass("java.lang.String")}
			case "allocation":
				receiver.IsThis = false
			case "other owner":
				call.ClassName = "proof.Other"
			case "ordinary method":
				ctx.FunctionName = "read"
			case "wrong invoke":
				call.Kind = InvokeVirtual
			case "primitive":
				call.Descriptor = "(I)V"
				table.Methods[0].Desc = call.Descriptor
			case "wrong arity":
				call.Descriptor = "()V"
			}
			if got := call.uniqueDelegationNullArgument(0, ctx); got != want {
				t.Fatalf("null applicability proof=%v want=%v", got, want)
			}
			if want && call.renderArgAt(0, ctx) != "null" {
				t.Fatal("unique original null must not acquire a source cast")
			}
			if variant == "same arity" && call.renderArgAt(0, ctx) == "null" {
				t.Fatal("competing reference overload still requires binding")
			}
		})
	}
}

func TestConstructorNullGenericBindingRequiresOriginalDeclarationScopes(t *testing.T) {
	for _, variant := range []string{"parameterized", "class formal", "method formal", "unbound", "wrong erasure", "malformed", "primitive rival", "reference rival", "hidden outer port", "missing lift", "wrong lift", "wrong lift owner", "caller method binder", "foreign receiver scope", "canceled", "budget"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Ljava/util/List;)V"
			table := callbinding.Class{Name: "proof/Owner", Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;", MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc, Signature: "(Ljava/util/List<TT;>;)V", Generic: true}}}
			ctx := &class_context.ClassContext{ClassName: "proof.Owner", FunctionName: "<init>"}
			receiver := NewJavaRef(nil, nil, types.NewJavaClass("proof.Owner"))
			receiver.IsThis = true
			call := &FunctionCallExpression{ClassName: "proof.Owner", FunctionName: "<init>", Descriptor: desc, Object: receiver, Arguments: []JavaValue{NewOriginalNullLiteral(5)}, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 6, HasOriginPC: true}
			want := variant == "parameterized" || variant == "class formal" || variant == "primitive rival" || variant == "hidden outer port"
			switch variant {
			case "class formal":
				table.Methods[0].Signature = "(TT;)V"
				table.Methods[0].Desc = "(Ljava/lang/Number;)V"
				call.Descriptor = table.Methods[0].Desc
			case "method formal":
				table.Methods[0].Signature = "<M:Ljava/lang/Object;>(Ljava/util/List<TM;>;)V"
			case "unbound":
				table.Methods[0].Signature = "(Ljava/util/List<TU;>;)V"
			case "wrong erasure":
				table.Methods[0].Signature = "(Ljava/util/Set<TT;>;)V"
			case "malformed":
				table.Methods[0].Signature = "(Ljava/util/List<TT;>;)Vx"
			case "primitive rival":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "(I)V"})
			case "reference rival":
				table.Methods = append(table.Methods, callbinding.Method{Name: "<init>", Desc: "(Ljava/util/Set;)V"})
			case "hidden outer port", "missing lift", "wrong lift", "wrong lift owner":
				table.Signature = ""
				ctx.LexicalTypeParamSignatures = []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}
				table.Methods[0].Desc = "(Lproof/Outer;Ljava/util/List;)V"
				call.Descriptor = table.Methods[0].Desc
				call.Arguments = []JavaValue{NewJavaRef(nil, nil, types.NewJavaClass("proof.Outer")), NewOriginalNullLiteral(5)}
				if variant != "missing lift" {
					ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
						sig := "(Lproof/Outer;Ljava/util/List<TT;>;)V"
						if variant == "wrong lift" {
							sig = "(Lproof/Outer;Ljava/util/Set<TT;>;)V"
						}
						return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("<init>", call.Descriptor): sig}, variant != "wrong lift owner" && name == table.Name
					}
				}
			case "caller method binder":
				table.Signature = ""
				ctx.CurrentMethodSig = "<T:Ljava/lang/Number;>()V"
			case "foreign receiver scope":
				table.Signature = ""
				ctx.LexicalTypeParamSignatures = []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}
				ctx.ClassName = "proof.Child"
				ctx.SupperClassName = "proof.Owner"
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) { return table, n == table.Name }
			index := len(call.Arguments) - 1
			if got := call.uniqueDelegationNullArgument(index, ctx); got != want {
				t.Fatalf("proof=%v want=%v", got, want)
			}
		})
	}
}
