package values

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestDeclaredArrayArgumentKeepsOriginalErasureAndSourceFamily(t *testing.T) {
	for _, variant := range []string{"plain", "bridge and ancestor", "concrete rival", "original checkcast", "original producer", "inferred producer is not physical array", "non-array JVM parameter", "forged parameter", "method formal", "shadowed formal", "wrong descriptor", "narrower array check", "foreign formal", "incomplete parent", "foreign receiver", "no origin", "static", "different ordinary return", "noncovariant ancestor", "different varargs rival", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			const sig = "<E:Ljava/lang/Object;:Ljava/lang/CharSequence;S:Lproof/Owner;>Ljava/lang/Object;"
			const desc = "([Ljava/lang/Object;)Lproof/Owner;"
			target := callbinding.Method{Name: "consume", Desc: desc, Signature: "([TE;)TS;", Generic: true, Public: true, Varargs: true}
			owner := callbinding.Class{Name: "proof/Owner", Signature: sig, Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{target}}
			classes := map[string]callbinding.Class{
				owner.Name:               owner,
				"java/lang/Object":       {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true},
				"java/lang/CharSequence": {Name: "java/lang/CharSequence", IsInterface: true, Public: true, MembersComplete: true, ParentsComplete: true},
				"java/lang/String":       {Name: "java/lang/String", Parents: []string{"java/lang/Object", "java/lang/CharSequence"}, Public: true, MembersComplete: true, ParentsComplete: true},
			}
			ctx := &class_context.ClassContext{ClassName: "proof.Owner", ClassSig: sig, ClassTypeParams: []string{"E", "S"}, TypeParams: []string{"E", "S"}, CurrentMethodSig: "([Ljava/lang/Object;)TS;", CurrentMethodDesc: desc, InvocationMetadata: func(n string) (callbinding.Class, bool) { c, known := classes[n]; return c, known }}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Owner"))
			receiver.IsThis, receiver.IsParam = true, true
			receiver.MarkOriginalParameter(0)
			receiver.MarkOriginalReceiver()
			arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
			arg.IsParam = true
			arg.MarkOriginalParameter(1)
			mt, err := types.ParseMethodDescriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			f := &FunctionCallExpression{ClassName: "proof.Owner", FunctionName: "consume", Descriptor: desc, Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 8, Object: receiver, Arguments: []JavaValue{arg}, FuncType: mt.FunctionType()}
			want := ""
			switch variant {
			case "plain":
				want = "E[]"
			case "original checkcast":
				f.Arguments[0] = NewOriginalCheckCast(arg, arg.Type(), 4)
				want = "E[]"
			case "original producer", "inferred producer is not physical array":
				producer, _ := types.ParseMethodDescriptor("()[Ljava/lang/Object;")
				child := &FunctionCallExpression{ClassName: "proof.Factory", FunctionName: "array", Descriptor: "()[Ljava/lang/Object;", FuncType: producer.FunctionType(), Kind: InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: 3}
				f.Arguments[0] = child
				if variant == "inferred producer is not physical array" {
					child.Descriptor = "()Ljava/lang/Object;"
				} else {
					want = "E[]"
				}
			case "non-array JVM parameter":
				ctx.CurrentMethodDesc = "(Ljava/lang/Object;)Lproof/Owner;"
			case "forged parameter":
				copy := NewJavaRef(utils.NewRootVariableId(), nil, arg.Type())
				copy.IsParam = true
				f.Arguments[0] = copy
			case "bridge and ancestor", "different ordinary return", "noncovariant ancestor":
				owner.Parents = append(owner.Parents, "proof/Contract")
				rival := callbinding.Method{Name: "consume", Desc: "([Ljava/lang/Object;)Ljava/lang/Object;", Public: true, Varargs: true, Generic: true, Signature: "([TE;)TS;"}
				classes["proof/Contract"] = callbinding.Class{Name: "proof/Contract", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{rival}}
				bridge := rival
				bridge.Bridge, bridge.Varargs, bridge.Generic, bridge.Signature = true, false, false, ""
				if variant == "different ordinary return" {
					bridge.Bridge = false
				} else if variant == "noncovariant ancestor" {
					c := classes["proof/Contract"]
					c.Methods[0].Desc = "([Ljava/lang/Object;)Ljava/lang/String;"
					classes[c.Name] = c
				} else {
					want = "E[]"
				}
				owner.Methods = append(owner.Methods, bridge)
			case "concrete rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "(Ljava/lang/Object;)Lproof/Owner;", Public: true})
				want = "E[]"
			case "method formal":
				owner.Methods[0].Signature = "<E:Ljava/lang/Object;>([TE;)TS;"
			case "shadowed formal":
				ctx.CurrentMethodSig = "<E:Ljava/lang/Number;>()V"
			case "wrong descriptor":
				owner.Methods[0].Signature = "([Ljava/lang/String;)TS;"
			case "narrower array check":
				ctx.ClassSig = "<E:Ljava/lang/CharSequence;S:Lproof/Owner;>Ljava/lang/Object;"
				owner.Signature = ctx.ClassSig
				f.Descriptor = "([Ljava/lang/CharSequence;)Lproof/Owner;"
				owner.Methods[0].Desc = f.Descriptor
				arg.ResetVarType(types.NewJavaArrayType(types.NewJavaClass("java.lang.CharSequence")))
			case "foreign formal":
				owner.Methods[0].Signature = "([TUnknown;)TS;"
			case "incomplete parent":
				owner.Parents = append(owner.Parents, "proof/Missing")
			case "foreign receiver":
				receiver.IsThis = false
			case "no origin":
				f.HasOriginPC = false
			case "static":
				f.IsStatic, f.Kind = true, InvokeStatic
			case "different varargs rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "([Ljava/lang/String;)Lproof/Owner;", Public: true, Varargs: true})
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			classes[owner.Name] = owner
			before, operand, identity := f.Witness(), f.Arguments[0].Type().Copy(), f.Arguments[0]
			if got := f.declaredArrayArgumentCast(0, ctx); got != want {
				t.Fatalf("cast=%q want=%q", got, want)
			}
			if f.Witness() != before || f.Object != receiver || f.Arguments[0] != identity || !reflect.DeepEqual(operand, identity.Type()) {
				t.Fatal("source-only view changed the original invocation or operand")
			}
		})
	}
}
