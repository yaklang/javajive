package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestAdversarialSourceDeadStoreNeedsCompleteLogicalReadClosure(t *testing.T) {
	for _, scenario := range []string{"dead literal", "dead null", "dead local load", "return read", "parameter", "other identity same spelling", "field receiver", "array target", "captured lambda", "effectful initializer", "cast initializer", "class literal initializer", "implicit narrowing", "primitive null", "opaque statement", "opaque value", "value cycle", "slot depth", "statement cycle", "invalid transfer", "typed nil", "work cap", "memory cap", "depth cap", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			id := utils.NewRootVariableId()
			id.SetName("same")
			ref := values.NewJavaRef(id, nil, typ)
			store := statements.NewAssignStatement(ref, values.NewJavaLiteral(7, typ), true)
			store.HasOriginPC = true
			store.OriginPC = 11
			branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{store}, nil)
			body := []statements.Statement{branch, statements.NewReturnStatement(nil)}
			params := []values.JavaValue{}
			var work *workbudget.Budget
			other := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			other.Id.SetName("same")
			switch scenario {
			case "dead null":
				ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
				store.JavaValue = values.JavaNull
			case "dead local load":
				store.JavaValue = other
			case "implicit narrowing":
				store.JavaValue = values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaLong))
			case "primitive null":
				store.JavaValue = values.JavaNull
			case "return read":
				body[1] = statements.NewReturnStatement(ref)
			case "parameter":
				params = append(params, ref)
			case "other identity same spelling":
				body[1] = statements.NewReturnStatement(other)
			case "field receiver":
				body[1] = statements.NewReturnStatement(&values.RefMember{Object: ref, Member: "field", JavaType: typ})
			case "array target":
				body[1] = &statements.AssignStatement{LeftValue: ref, ArrayMember: &values.JavaArrayMember{Object: ref, Index: values.NewJavaLiteral(0, typ)}, JavaValue: values.JavaNull}
			case "captured lambda":
				body[1] = statements.NewReturnStatement(&values.CustomValue{Flag: "lambda", CapturesKnown: true, Captures: []values.JavaValue{ref}})
			case "effectful initializer":
				store.JavaValue = &values.FunctionCallExpression{FunctionName: "event", ClassName: "Effects", Descriptor: "()I", IsStatic: true, Kind: values.InvokeStatic}
			case "cast initializer":
				store.JavaValue = &values.CastExpression{Value: other, TargetType: typ, OriginPC: 10}
			case "class literal initializer":
				store.JavaValue = values.NewJavaClassValue(types.NewJavaClass("unresolved.Symbol"))
			case "opaque statement":
				body = append(body, statements.NewCustomStatement(func(*class_context.ClassContext) string { return "same()" }, nil))
			case "opaque value":
				body[1] = statements.NewReturnStatement(values.NewCustomValue(func(*class_context.ClassContext) string { return "same" }, nil))
			case "value cycle":
				x := &values.CustomValue{Flag: "lambda", CapturesKnown: true}
				x.Captures = []values.JavaValue{x}
				body[1] = statements.NewReturnStatement(x)
			case "slot depth":
				var x values.JavaValue = other
				for i := 0; i < 130; i++ {
					x = values.NewSlotValue(x, typ)
				}
				store.JavaValue = x
			case "statement cycle":
				branch.IfBody = append(branch.IfBody, branch)
			case "invalid transfer":
				x := statements.NewSourceTransferStatement("break", "")
				x.LoopTargetLabel = "x;hidden()"
				x.Name = "break"
				body = append(body, x)
			case "typed nil":
				var missing *statements.AssignStatement
				body = append(body, missing)
			case "work cap":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory cap":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "depth cap":
				work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := sourceWithoutDeadLocalStores(body, params, work)
			want := scenario == "dead literal" || scenario == "dead null" || scenario == "dead local load" || scenario == "other identity same spelling"
			result := got[0].(*statements.IfStatement)
			removed := len(result.IfBody) == 0
			if removed != want {
				t.Fatalf("removed=%v", removed)
			}
			if len(branch.IfBody) == 0 || branch.IfBody[0] != store {
				t.Fatal("original graph mutated")
			}
			if removed && (result == branch || store.LeftValue != ref || ref.Id != id) {
				t.Fatal("source view changed binding identity")
			}
			if !removed && got[0] != branch {
				t.Fatal("unproved rewrite changed original tree")
			}
		})
	}
}
