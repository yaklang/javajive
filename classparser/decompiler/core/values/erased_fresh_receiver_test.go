package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestErasedFreshReceiverRequiresClassArgumentAndProducerDeclarations(t *testing.T) {
	for _, variant := range []string{"proved", "renamed", "missing producer origin", "bad producer origin", "argument count", "method inference", "raw producer return", "mismatched signature", "missing producer declaration", "nondependent input", "method dependent input", "missing input signature", "varargs", "bridge", "wrong owner", "incomplete owner", "unproved consumer", "duplicate target"} {
		t.Run(variant, func(t *testing.T) {
			getter, _, ctx, classes := erasedZeroArgumentFixture()
			producer := getter.Object.(*FunctionCallExpression)
			factory := classes["proof/Factory"]
			factory.Methods[0].Signature = "([Ljava/lang/Object;)Lproof/Cell<Ljava/lang/String;>;"
			cell := classes["proof/Cell"]
			desc := "(Ljava/lang/Object;)Lproof/Cell;"
			cell.Methods = []callbinding.Method{{Name: "store", Desc: desc, Signature: "(TV;)Lproof/Cell<TV;>;", Generic: true, Public: true}}
			ft, _ := types.ParseMethodDescriptor(desc)
			f := &FunctionCallExpression{ClassName: "proof.Cell", FunctionName: "store", Descriptor: desc, FuncType: ft.FunctionType(), Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 13, Object: producer, Arguments: []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))}}
			switch variant {
			case "renamed":
				f.FunctionName = "append"
				cell.Methods[0].Name = f.FunctionName
			case "missing producer origin":
				producer.HasOriginPC = false
			case "bad producer origin":
				producer.OriginPC = -1
			case "argument count":
				producer.Arguments = nil
			case "method inference":
				factory.Methods[0].Signature = "<X:Ljava/lang/Object;>([TX;)Lproof/Cell<TX;>;"
			case "raw producer return":
				factory.Methods[0].Signature = "([Ljava/lang/Object;)Lproof/Cell;"
			case "mismatched signature":
				factory.Methods[0].Signature = "()Lproof/Cell<Ljava/lang/String;>;"
			case "missing producer declaration":
				factory.Methods = nil
			case "nondependent input":
				cell.Methods[0].Signature = "(Ljava/lang/Object;)Lproof/Cell<TV;>;"
			case "method dependent input":
				cell.Methods[0].Signature = "<V:Ljava/lang/Object;>(TV;)Lproof/Cell;"
			case "missing input signature":
				cell.Methods[0].Signature = ""
			case "varargs":
				cell.Methods[0].Varargs = true
			case "bridge":
				cell.Methods[0].Bridge = true
			case "wrong owner":
				cell.Name = "other/Cell"
			case "incomplete owner":
				cell.MembersComplete = false
			case "unproved consumer":
				f.HasOriginPC = false
			case "duplicate target":
				cell.Methods = append(cell.Methods, cell.Methods[0])
			}
			classes["proof/Cell"], classes[factory.Name] = cell, factory
			got := f.freshReceiverClassArgumentConstraints(cell, ctx)
			if got != (variant == "proved" || variant == "renamed") {
				t.Fatalf("class argument obligation=%v", got)
			}
		})
	}
}
