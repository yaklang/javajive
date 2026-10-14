package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestDelegationCompletedOperandReopensOriginalArrayTransaction(t *testing.T) {
	for _, variant := range []string{"original", "changed runtime component", "changed element", "changed index", "wrong end", "missing origin", "unknown origin", "missing end", "wrong allocation identity", "missing duplicate", "wrong duplicate identity", "local publication", "field publication", "other use", "copied source array", "wrong source formal", "wrong physical formal", "wrong receiver", "wrong call binding", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
			formal := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
			item := values.NewJavaLiteral("value", types.NewJavaClass("java.lang.String"))
			array := values.NewNewArrayExpression(arrayType, values.NewJavaLiteral(1, integer))
			array.Initializer = []values.JavaValue{item}
			array.HasOriginPC, array.OriginPC = true, 10
			array.HasEvaluationEndPC, array.EvaluationEndPC = true, 15
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Child"))
			receiver.IsThis = true
			call := &values.FunctionCallExpression{ClassName: "proof.Child", FunctionName: "<init>", Descriptor: "([Ljava/lang/Object;)V", Kind: values.InvokeSpecial, Object: receiver, Arguments: []values.JavaValue{array}, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaVoid), ParamTypes: []types.JavaType{formal}}}
			allocate := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_ANEWARRAY}, stackProduced: []values.JavaValue{array}}
			dup := &OpCode{CurrentOffset: 11, Instr: &Instruction{OpCode: OP_DUP}, stackConsumed: []values.JavaValue{array}, stackProduced: []values.JavaValue{ref, ref}}
			allocate.Target = []*OpCode{dup}
			store := &OpCode{CurrentOffset: 15, Instr: &Instruction{OpCode: OP_AASTORE}, stackConsumed: []values.JavaValue{item, values.NewJavaLiteral(0, integer), ref}}
			invoke := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, stackConsumed: []values.JavaValue{ref, receiver}}
			d := &Decompiler{opCodes: []*OpCode{allocate, dup, store, invoke}, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{allocate: nil, dup: nil, store: nil, invoke: nil}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			before := append([]values.JavaValue(nil), invoke.stackConsumed...)
			switch variant {
			case "changed runtime component":
				array.JavaType = formal
			case "changed element":
				array.Initializer[0] = values.NewJavaLiteral("different", types.NewJavaClass("java.lang.String"))
			case "changed index":
				store.stackConsumed[1] = values.NewJavaLiteral(1, integer)
			case "wrong end":
				array.EvaluationEndPC++
			case "missing origin":
				array.HasOriginPC = false
			case "unknown origin":
				array.OriginPC = 12
			case "missing end":
				array.HasEvaluationEndPC = false
			case "wrong allocation identity":
				allocate.stackProduced[0] = values.JavaNull
			case "missing duplicate":
				allocate.Target = nil
			case "wrong duplicate identity":
				dup.stackProduced[1] = values.JavaNull
			case "local publication", "field publication", "other use":
				kind := OP_ASTORE_1
				if variant == "field publication" {
					kind = OP_PUTSTATIC
				} else if variant == "other use" {
					kind = OP_INVOKESTATIC
				}
				d.opCodes = append(d.opCodes, &OpCode{CurrentOffset: 17, Instr: &Instruction{OpCode: kind}, stackConsumed: []values.JavaValue{ref}})
			case "copied source array":
				copy := *array
				call.Arguments[0] = &copy
			case "wrong source formal":
				call.FuncType.ParamTypes[0] = arrayType
			case "wrong physical formal":
				call.Descriptor = "([Ljava/lang/String;)V"
			case "wrong receiver":
				invoke.stackConsumed[1] = values.JavaNull
			case "wrong call binding":
				other := *call
				other.Descriptor = "(Ljava/lang/Object;)V"
				d.invokeFuncCall[invoke] = &other
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.delegationArrayCallOperands(call, invoke); got != (variant == "original") {
				t.Fatalf("completed operand identity=%v", got)
			}
			if variant != "wrong receiver" {
				for i, original := range before {
					if invoke.stackConsumed[i] != original {
						t.Fatal("operand proof rewrote the original stack")
					}
				}
			}
		})
	}
}
