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

func referenceBindingMetadataFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class, map[string][]string) {
	declarations := map[string]callbinding.Class{
		"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true},
		"proof/Contract":   {Name: "proof/Contract", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}},
		"proof/Dispatcher": {Name: "proof/Dispatcher", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{
			{Name: "pick", Desc: "(Lproof/Contract;)I", Public: true, Static: true},
			{Name: "pick", Desc: "(Lproof/Child;)I", Public: true, Static: true},
		}},
	}
	parents := map[string][]string{"proof/Child": {"proof/Contract"}}
	ctx := &class_context.ClassContext{IsStatic: true, ClassName: "proof/Caller", CurrentMethodDesc: "(Ljava/lang/Object;)I",
		InvocationMetadata: func(name string) (callbinding.Class, bool) { c, ok := declarations[name]; return c, ok },
		SiblingSuperTypes:  func(name string) ([]string, bool) { p, ok := parents[name]; return p, ok },
	}
	parameter := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	parameter.IsParam = true
	parameter.MarkOriginalParameter(0)
	method, _ := types.ParseMethodDescriptor("(Lproof/Contract;)I")
	call := &FunctionCallExpression{ClassName: "proof.Dispatcher", FunctionName: "pick", Descriptor: "(Lproof/Contract;)I", FuncType: method.FunctionType(), IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 9,
		Arguments: []JavaValue{NewOriginalCheckCast(parameter, types.NewJavaClass("proof.Child"), 4)},
	}
	return call, ctx, declarations, parents
}

func TestReferenceBindingAncestryOnlyProvesFixedWideningViews(t *testing.T) {
	for _, variant := range []string{"original check", "original parameter", "array check", "receiver and argument", "unknown access callback"} {
		t.Run(variant, func(t *testing.T) {
			call, ctx, declarations, parents := referenceBindingMetadataFixture()
			if variant == "original parameter" {
				parameter := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Child"))
				parameter.IsParam = true
				parameter.MarkOriginalParameter(0)
				ctx.CurrentMethodDesc = "(Lproof/Child;)I"
				call.Arguments[0] = parameter
			}
			if variant == "array check" {
				child, _ := types.ParseDescriptor("[Lproof/Child;")
				call.Arguments[0] = NewOriginalCheckCast(call.Arguments[0].(*CastExpression).Value, child, 4)
				call.Descriptor = "([Lproof/Contract;)I"
				method, _ := types.ParseMethodDescriptor(call.Descriptor)
				call.FuncType = method.FunctionType()
				c := declarations["proof/Dispatcher"]
				c.Methods[0].Desc, c.Methods[1].Desc = call.Descriptor, "([Lproof/Child;)I"
				declarations[c.Name] = c
			}
			if variant == "receiver and argument" {
				call.Object = call.Arguments[0]
				call.IsStatic, call.Kind = false, InvokeVirtual
				c := declarations["proof/Dispatcher"]
				for i := range c.Methods {
					c.Methods[i].Static = false
				}
				declarations[c.Name] = c
				parents["proof/Child"] = []string{"proof/Contract", "proof/Dispatcher"}
			}
			if variant == "unknown access callback" {
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, false }
			}
			before := call.Clone()
			planned, known := call.planCallBinding(ctx)
			if !known || planned.Witness() != call.Witness() || ctx.OverloadFamilyUnproven {
				t.Fatal("complete declaration + original ancestry did not preserve its binding")
			}
			view, ok := planned.Arguments[0].(*CastExpression)
			if !ok || !view.Binding || view.Value != call.Arguments[0] || !reflect.DeepEqual(before, call) {
				t.Fatal("widening view changed shared operands, checks or dispatch")
			}
			if variant == "receiver and argument" && planned.Object.(*CastExpression).Value != call.Object {
				t.Fatal("receiver identity changed")
			}
			if _, known := ctx.InvocationMetadata("proof/Child"); known {
				t.Fatal("inheritance evidence was published as a complete declaration")
			}
		})
	}
}

func TestReferenceBindingAncestryCannotInventMemberOrOperandPermissions(t *testing.T) {
	for _, variant := range []string{"missing ancestry", "missing side branch", "cycle", "too many parents", "wrong declaration identity", "incomplete declaration", "unknown owner", "unknown inherited members", "private owner", "private formal", "private method", "generic method", "bridge method", "varargs method", "wrong static flag", "source access denial", "missing origin", "late check", "wrong kind", "type model drift", "return model drift", "synthetic cast", "unsealed cast", "mutated check", "inferred local", "unproved parameter", "wrong parameter slot", "poly producer", "work", "memory", "copy budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			call, ctx, declarations, parents := referenceBindingMetadataFixture()
			owner := declarations["proof/Dispatcher"]
			cast := call.Arguments[0].(*CastExpression)
			switch variant {
			case "missing ancestry":
				ctx.SiblingSuperTypes = nil
			case "missing side branch":
				parents["proof/Child"] = []string{"proof/Contract", "missing/Other"}
			case "cycle":
				parents["proof/Child"] = []string{"proof/Contract", "proof/Child"}
			case "too many parents":
				parents["proof/Child"] = make([]string, 1025)
				for i := range parents["proof/Child"] {
					parents["proof/Child"][i] = "proof/Contract"
				}
			case "wrong declaration identity":
				declarations["proof/Child"] = callbinding.Class{Name: "proof/Other"}
			case "incomplete declaration":
				declarations["proof/Child"] = callbinding.Class{Name: "proof/Child"}
			case "unknown owner":
				delete(declarations, owner.Name)
				parents[owner.Name] = []string{"java/lang/Object"}
			case "unknown inherited members":
				owner.Parents = append(owner.Parents, "proof/Ancestor")
				parents["proof/Ancestor"] = []string{"java/lang/Object"}
			case "private owner":
				owner.Public = false
			case "private formal":
				c := declarations["proof/Contract"]
				c.Public = false
				declarations[c.Name] = c
			case "private method":
				owner.Methods[0].Public = false
			case "generic method":
				owner.Methods[0].Generic = true
			case "bridge method":
				owner.Methods[0].Bridge = true
			case "varargs method":
				owner.Methods[0].Varargs = true
			case "wrong static flag":
				owner.Methods[0].Static = false
			case "source access denial":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, true }
			case "missing origin":
				call.HasOriginPC = false
			case "late check":
				cast.OriginPC = call.OriginPC
			case "wrong kind":
				call.Kind = InvokeVirtual
			case "type model drift":
				call.FuncType.ParamTypes[0] = types.NewJavaClass("java.lang.Object")
			case "return model drift":
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.Object")
			case "synthetic cast":
				cast.Binding = true
			case "unsealed cast":
				call.Arguments[0] = &CastExpression{Value: cast.Value, TargetType: cast.TargetType, OriginalCheckCast: true, OriginPC: 4}
			case "mutated check":
				cast.TargetType = types.NewJavaClass("proof.Unrelated")
			case "inferred local", "unproved parameter", "wrong parameter slot":
				ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Child"))
				if variant != "inferred local" {
					ref.IsParam = true
				}
				if variant == "wrong parameter slot" {
					ref.MarkOriginalParameter(3)
				}
				call.Arguments[0] = ref
			case "poly producer":
				call.Arguments[0] = &FunctionCallExpression{ClassName: "proof.Factory", FunctionName: "make", Descriptor: "()Lproof/Child;", IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 4, FuncType: &types.JavaFuncType{ReturnType: cast.TargetType}}
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "copy budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxNodeCopies: 1})
			case "canceled":
				cancel, stop := context.WithCancel(context.Background())
				stop()
				ctx.Work = workbudget.New(cancel, workbudget.Limits{})
			}
			if variant != "unknown owner" {
				declarations[owner.Name] = owner
			}
			if _, known := call.planCallBinding(ctx); known {
				t.Fatal("ancestry evidence granted an unproved call/source permission")
			}
		})
	}
}
