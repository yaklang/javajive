package values

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestErasedConcreteArgumentRequiresOriginalConsumerAndResultProof(t *testing.T) {
	for _, variant := range []string{"exact", "renamed", "missing PC", "bad PC", "missing metadata", "incomplete family", "different owner", "different source type", "generic consumer", "method signature", "competing arity", "other arity", "varargs", "bridge", "wrong static", "special", "narrower result", "changed producer", "checked producer", "work", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			getter, f, ctx, classes := erasedZeroArgumentFixture()
			f.IsStatic, f.Kind = true, InvokeStatic
			f.Descriptor = "(ILjava/lang/Object;)I"
			ft, _ := types.ParseMethodDescriptor(f.Descriptor)
			f.FuncType = ft.FunctionType()
			f.Object = nil
			f.Arguments = []JavaValue{NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)), getter}
			c := classes["proof/Owner"]
			c.Methods = []callbinding.Method{{Name: f.FunctionName, Desc: f.Descriptor, Public: true, Static: true, ExceptionsKnown: true}}
			switch variant {
			case "renamed":
				f.FunctionName = "measure"
				c.Methods[0].Name = f.FunctionName
			case "missing PC":
				f.HasOriginPC = false
			case "bad PC":
				f.OriginPC = 65536
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "incomplete family":
				c.ParentsComplete = false
			case "different owner":
				c.Name = "unrelated/Owner"
			case "different source type":
				f.FuncType.ParamTypes[1] = types.NewJavaClass("java.lang.String")
			case "generic consumer":
				c.Methods[0].Generic = true
			case "method signature":
				c.Methods[0].Signature = "<E:Ljava/lang/Object;>(ITE;)I"
			case "competing arity":
				c.Methods = append(c.Methods, callbinding.Method{Name: f.FunctionName, Desc: "(ILjava/util/List;)I", Public: true, Static: true})
			case "other arity":
				c.Methods = append(c.Methods, callbinding.Method{Name: f.FunctionName, Desc: "(I)I", Public: true, Static: true})
			case "varargs":
				c.Methods[0].Varargs = true
			case "bridge":
				c.Methods[0].Bridge = true
			case "wrong static":
				c.Methods[0].Static = false
			case "special":
				f.IsSpecialInvoke = true
			case "narrower result":
				f.Descriptor = "(ILjava/lang/String;)I"
				c.Methods[0].Desc = f.Descriptor
				ft, _ = types.ParseMethodDescriptor(f.Descriptor)
				f.FuncType = ft.FunctionType()
			case "changed producer":
				getter.HasOriginPC = false
			case "checked producer":
				m := classes["proof/Cell"]
				m.Methods[0].Exceptions = []string{"java/io/IOException"}
				classes[m.Name] = m
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				base, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(base, workbudget.Limits{})
			}
			classes["proof/Owner"] = c
			before := f.Clone()
			witness := f.Witness()
			got := f.erasedConcreteArgumentResultView(1, ctx)
			if (got != nil) != (variant == "exact" || variant == "renamed" || variant == "other arity") {
				t.Fatalf("erased argument view=%v", got)
			}
			if !reflect.DeepEqual(f, before) || f.Witness() != witness {
				t.Fatal("mutated shared consumer")
			}
			if got != nil {
				cast, ok := got.(*CastExpression)
				if !ok || bindingType(cast.TargetType) != "Ljava/lang/Object;" || cast.Value.(*FunctionCallExpression).Witness() != getter.Witness() {
					t.Fatal("changed original result/consumer tuple")
				}
			}
		})
	}
}

func TestErasedResultChainIncludesBoundReceiverConstraints(t *testing.T) {
	for _, variant := range []string{"exact", "raw receiver", "zero-input factory", "incomplete", "duplicate", "wrong metadata", "narrow argument"} {
		t.Run(variant, func(t *testing.T) {
			_, _, ctx, classes := erasedZeroArgumentFixture()
			desc := "(Ljava/lang/Object;)Lproof/Cell;"
			c := classes["proof/Cell"]
			c.Methods = []callbinding.Method{{Name: "store", Desc: desc, Signature: "(TV;)Lproof/Cell<TV;>;", Generic: true, Public: true}}
			classes[c.Name] = c
			ft, _ := types.ParseMethodDescriptor(desc)
			f := &FunctionCallExpression{ClassName: "proof.Cell", FunctionName: "store", Descriptor: desc, FuncType: ft.FunctionType(), Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 13, Object: NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("proof.Cell", []types.JavaType{types.NewJavaClass("java.lang.String")})), Arguments: []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))}}
			switch variant {
			case "zero-input factory":
				f.Descriptor = "()Lproof/Cell;"
				ft, _ = types.ParseMethodDescriptor(f.Descriptor)
				f.FuncType = ft.FunctionType()
				f.Arguments = nil
				c.Methods[0].Desc = f.Descriptor
				c.Methods[0].Signature = "()Lproof/Cell<TV;>;"
			case "raw receiver":
				f.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Cell"))
			case "incomplete":
				c.MembersComplete = false
			case "duplicate":
				c.Methods = append(c.Methods, c.Methods[0])
			case "wrong metadata":
				c.Name = "unrelated/Cell"
			case "narrow argument":
				f.Descriptor = "(Ljava/lang/String;)Lproof/Cell;"
			}
			classes["proof/Cell"] = c
			out, ok := f.PlanErasedResultChain(ctx, "Lproof/Cell;")
			if ok != (variant == "exact") {
				t.Fatalf("receiver erasure planned=%v", ok)
			}
			if ok && (out.Witness() != f.Witness() || out.Object.(*CastExpression).Value != f.Object || out.Arguments[0].(*CastExpression).Value != f.Arguments[0]) {
				t.Fatal("erasure changed runtime operands")
			}
		})
	}
}
