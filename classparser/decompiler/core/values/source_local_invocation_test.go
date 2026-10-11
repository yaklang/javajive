package values

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSourceLocalInvocationUsesOnlyBoundedCertifiedProducerView(t *testing.T) {
	for _, variant := range []string{"original", "missing callback", "unknown certificate", "foreign source method", "noncall certificate", "typed nil call", "missing producer origin", "foreign producer declaration", "wrong return word", "cyclic producer", "nested uncertified local", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			getterDesc, getterSig := "()Lprobe/Parent;", "()Lprobe/Parent<Ljava/lang/String;>;"
			outerDesc, outerSig := "(Lprobe/Parent;)Lprobe/Box;", "<T:Ljava/lang/Object;>(Lprobe/Parent<TT;>;)Lprobe/Box<TT;>;"
			metas := map[string]callbinding.Class{
				"probe/Factory": {Name: "probe/Factory", ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "get", Desc: getterDesc, Signature: getterSig, Static: true, Public: true}, {Name: "wrap", Desc: outerDesc, Signature: outerSig, Static: true, Generic: true, Public: true}}},
				"probe/Parent":  {Name: "probe/Parent", ParentsComplete: true, MembersComplete: true, Parents: []string{"java/lang/Object"}, Signature: "<E:Ljava/lang/Object;>Ljava/lang/Object;"},
			}
			ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("probe.Parent"))
			getter := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "get", Descriptor: getterDesc, IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 3, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Parent")}}
			outer := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "wrap", Descriptor: outerDesc, IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 9, Arguments: []JavaValue{ref}, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Box")}}
			ctx := &class_context.ClassContext{ClassName: "probe.Owner", FunctionName: "make", CurrentMethodDesc: "()V", InvocationMetadata: func(n string) (callbinding.Class, bool) { m, ok := metas[n]; return m, ok }}
			queries := 0
			ctx.SourceLocalInvocation = func(value any, owner, name, desc string) (any, bool) {
				queries++
				if variant == "noncall certificate" {
					return JavaNull, true
				}
				if variant == "typed nil call" {
					return (*FunctionCallExpression)(nil), true
				}
				return getter, value == ref && owner == "probe.Owner" && name == "make" && desc == "()V" && variant != "unknown certificate"
			}
			switch variant {
			case "missing callback":
				ctx.SourceLocalInvocation = nil
			case "foreign source method":
				ctx.FunctionName = "other"
			case "missing producer origin":
				getter.HasOriginPC = false
			case "foreign producer declaration":
				getter.ClassName = "probe.Foreign"
			case "wrong return word":
				m := metas["probe/Factory"]
				m.Methods[0].Desc, m.Methods[0].Signature = "()Lprobe/Box;", "()Lprobe/Box<Ljava/lang/String;>;"
				metas["probe/Factory"] = m
				getter.Descriptor, getter.FuncType.ReturnType = m.Methods[0].Desc, types.NewJavaClass("probe.Box")
			case "cyclic producer":
				getter.IsStatic, getter.Kind, getter.Object = false, InvokeVirtual, getter
			case "nested uncertified local":
				m := metas["probe/Factory"]
				m.Methods[0].Desc, m.Methods[0].Signature = "(Lprobe/Parent;)Lprobe/Parent;", "<E:Ljava/lang/Object;>(Lprobe/Parent<TE;>;)Lprobe/Parent<TE;>;"
				metas["probe/Factory"] = m
				getter.Descriptor, getter.Arguments = m.Methods[0].Desc, []JavaValue{ref}
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(canceled, workbudget.Limits{})
			}
			got := outer.SourceInvocationResultType(ctx)
			if (got != nil) != (variant == "original") || got != nil && got.String(ctx) != "Box<String>" {
				t.Fatalf("%s: certified local returned %v", variant, got)
			}
			if queries > 1 || outer.Arguments[0] != ref || ref.Type().String(ctx) != "Parent" || outer.FuncType.ReturnType.String(ctx) != "Box" {
				t.Fatal("producer proof recursed or changed original computational values")
			}
		})
	}
}
