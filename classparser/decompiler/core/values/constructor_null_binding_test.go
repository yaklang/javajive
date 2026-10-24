package values

import (
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
