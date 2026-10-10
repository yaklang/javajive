package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestPrivateDelegationArrayArgumentPositionRequiresPhysicalOperandIdentity(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
	for position := 0; position < 4; position++ {
		for _, variant := range []string{"original", "duplicate use", "wrong raw order", "wrong physical array type", "wrong source array type", "missing binding"} {
			t.Run(variant+string(rune('0'+position)), func(t *testing.T) {
				array := values.NewNewArrayExpression(arrayType, values.NewJavaLiteral(2, integer))
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
				receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Child"))
				receiver.IsThis = true
				call := &values.FunctionCallExpression{Object: receiver, ClassName: "proof.Base", FunctionName: "<init>", Kind: values.InvokeSpecial, IsSpecialInvoke: true, OriginPC: 30, HasOriginPC: true, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaVoid)}}
				params := ""
				for i := 0; i < 4; i++ {
					if i == position {
						call.Arguments = append(call.Arguments, ref)
						call.FuncType.ParamTypes = append(call.FuncType.ParamTypes, arrayType)
						params += "[Ljava/lang/String;"
					} else {
						call.Arguments = append(call.Arguments, values.NewJavaLiteral(i, integer))
						call.FuncType.ParamTypes = append(call.FuncType.ParamTypes, integer)
						params += "I"
					}
				}
				call.Descriptor = "(" + params + ")V"
				invoke := &OpCode{CurrentOffset: 30, Instr: &Instruction{OpCode: OP_INVOKESPECIAL}}
				for i := len(call.Arguments) - 1; i >= 0; i-- {
					invoke.stackConsumed = append(invoke.stackConsumed, call.Arguments[i])
				}
				invoke.stackConsumed = append(invoke.stackConsumed, receiver)
				d := &Decompiler{invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
				switch variant {
				case "duplicate use":
					call.Arguments[(position+1)%4] = ref
					invoke.stackConsumed[len(call.Arguments)-1-(position+1)%4] = ref
				case "wrong raw order":
					invoke.stackConsumed[len(call.Arguments)-1-position] = receiver
				case "wrong physical array type":
					call.Descriptor = replacePositionDescriptor(position)
				case "wrong source array type":
					copy := *call.FuncType
					copy.ParamTypes = append([]types.JavaType(nil), copy.ParamTypes...)
					copy.ParamTypes[position] = types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
					call.FuncType = &copy
				case "missing binding":
					delete(d.invokeFuncCall, invoke)
				}
				consumer, op, index, known := d.privateDelegationArrayConsumer(call, invoke, ref, array)
				if known != (variant == "original") || known && (consumer != call || op != invoke || index != position) {
					t.Fatalf("position %d: accepted=%v original index=%d", position, known, index)
				}
			})
		}
	}
}

func replacePositionDescriptor(position int) string {
	desc := "("
	for i := 0; i < 4; i++ {
		if i == position {
			desc += "[Ljava/lang/Object;"
		} else {
			desc += "I"
		}
	}
	return desc + ")V"
}

func TestPrivateDelegationCompletedArrayRejectsPublicationAndFalseStoreWitnesses(t *testing.T) {
	for _, variant := range []string{"original", "missing DUP", "wrong DUP value", "wrong consumed index", "wrong value", "wrong end", "incomplete initializer", "local publication", "field publication", "other invocation", "later reuse", "wrong source formal", "wrong physical formal"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			arrayType := types.NewJavaArrayType(integer)
			item := values.NewJavaLiteral(7, integer)
			array := values.NewNewArrayExpression(arrayType, values.NewJavaLiteral(1, integer))
			array.Initializer = []values.JavaValue{item}
			array.OriginPC, array.HasOriginPC = 10, true
			array.EvaluationEndPC, array.HasEvaluationEndPC = 15, true
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			node := NewNode(statements.NewAssignStatement(ref, array, true))
			node.Id = 11
			allocate := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_NEWARRAY}, stackProduced: []values.JavaValue{array}}
			dup := &OpCode{CurrentOffset: 11, Instr: &Instruction{OpCode: OP_DUP}, stackConsumed: []values.JavaValue{array}, stackProduced: []values.JavaValue{ref, ref}}
			allocate.Target = []*OpCode{dup}
			store := &OpCode{CurrentOffset: 15, Instr: &Instruction{OpCode: OP_IASTORE}, stackConsumed: []values.JavaValue{item, values.NewJavaLiteral(0, integer), ref}}
			invoke := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, stackConsumed: []values.JavaValue{ref}}
			call := &values.FunctionCallExpression{Descriptor: "([I)V", Arguments: []values.JavaValue{ref}, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{arrayType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}}
			d := &Decompiler{opCodes: []*OpCode{allocate, dup, store, invoke}, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{allocate: nil, dup: nil, store: nil, invoke: nil}}
			origins := map[int]*OpCode{node.Id: dup}
			switch variant {
			case "missing DUP":
				delete(origins, node.Id)
			case "wrong DUP value":
				dup.stackProduced[0] = item
			case "wrong consumed index":
				store.stackConsumed[1] = item
			case "wrong value":
				store.stackConsumed[0] = values.NewJavaLiteral(8, integer)
			case "wrong end":
				array.EvaluationEndPC = 16
			case "incomplete initializer":
				array.Initializer = append(array.Initializer, item)
			case "local publication", "field publication", "other invocation", "later reuse":
				opcode := OP_ASTORE_2
				if variant == "field publication" {
					opcode = OP_PUTSTATIC
				} else if variant == "other invocation" {
					opcode = OP_INVOKESTATIC
				}
				d.opCodes = append(d.opCodes, &OpCode{CurrentOffset: 19, Instr: &Instruction{OpCode: opcode}, stackConsumed: []values.JavaValue{ref}})
			case "wrong source formal":
				call.FuncType.ParamTypes[0] = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaLong))
			case "wrong physical formal":
				call.Descriptor = "([J)V"
			}
			if got := d.privateDelegationCompletedArray(delegationArrayMaterialization{node: node, array: array, ref: ref}, call, invoke, 0, origins); got != (variant == "original") {
				t.Fatalf("retained completed initializer=%v", got)
			}
		})
	}
}

func TestPrivateDelegationNestedArrayKeepsEveryChildPrivate(t *testing.T) {
	for _, variant := range []string{"original", "child local publication", "child field publication", "child extra call", "child later reuse", "child wrong index", "child wrong end", "child missing origin", "child false DUP", "child cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			literal := func(i int) *values.JavaLiteral { return values.NewJavaLiteral(i, integer) }
			childType := types.NewJavaArrayType(integer)
			parentType := types.NewJavaArrayType(childType)
			child := values.NewNewArrayExpression(childType, literal(1))
			child.Initializer = []values.JavaValue{literal(7)}
			child.OriginPC, child.HasOriginPC = 12, true
			child.EvaluationEndPC, child.HasEvaluationEndPC = 14, true
			parent := values.NewNewArrayExpression(parentType, literal(1))
			parent.Initializer = []values.JavaValue{child}
			parent.OriginPC, parent.HasOriginPC = 10, true
			parent.EvaluationEndPC, parent.HasEvaluationEndPC = 15, true
			childRef := values.NewJavaRef(utils.NewRootVariableId(), nil, childType)
			parentRef := values.NewJavaRef(utils.NewRootVariableId(), nil, parentType)
			node := NewNode(statements.NewAssignStatement(parentRef, parent, true))
			node.Id = 11
			parentAlloc := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_ANEWARRAY}, stackProduced: []values.JavaValue{parent}}
			parentDup := &OpCode{CurrentOffset: 11, Instr: &Instruction{OpCode: OP_DUP}, stackConsumed: []values.JavaValue{parent}, stackProduced: []values.JavaValue{parentRef, parentRef}}
			childAlloc := &OpCode{CurrentOffset: 12, Instr: &Instruction{OpCode: OP_NEWARRAY}, stackProduced: []values.JavaValue{child}}
			childDup := &OpCode{CurrentOffset: 13, Instr: &Instruction{OpCode: OP_DUP}, stackConsumed: []values.JavaValue{child}, stackProduced: []values.JavaValue{childRef, childRef}}
			childStore := &OpCode{CurrentOffset: 14, Instr: &Instruction{OpCode: OP_IASTORE}, stackConsumed: []values.JavaValue{child.Initializer[0], literal(0), childRef}}
			parentStore := &OpCode{CurrentOffset: 15, Instr: &Instruction{OpCode: OP_AASTORE}, stackConsumed: []values.JavaValue{childRef, literal(0), parentRef}}
			invoke := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, stackConsumed: []values.JavaValue{parentRef}}
			parentAlloc.Target, childAlloc.Target = []*OpCode{parentDup}, []*OpCode{childDup}
			ops := []*OpCode{parentAlloc, parentDup, childAlloc, childDup, childStore, parentStore, invoke}
			d := &Decompiler{opCodes: ops, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}}
			for _, op := range ops {
				d.opcodeToSimulateStack[op] = nil
			}
			call := &values.FunctionCallExpression{Descriptor: "([[I)V", Arguments: []values.JavaValue{parentRef}, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{parentType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}}
			switch variant {
			case "child local publication", "child field publication", "child extra call", "child later reuse":
				opcode, pc := OP_ASTORE_2, 16
				if variant == "child field publication" {
					opcode = OP_PUTSTATIC
				} else if variant == "child extra call" {
					opcode = OP_INVOKESTATIC
				} else if variant == "child later reuse" {
					opcode, pc = OP_AALOAD, 21
				}
				d.opCodes = append(d.opCodes, &OpCode{CurrentOffset: uint16(pc), Instr: &Instruction{OpCode: opcode}, stackConsumed: []values.JavaValue{childRef}})
			case "child wrong index":
				childStore.stackConsumed[1] = literal(1)
			case "child wrong end":
				child.EvaluationEndPC = 13
			case "child missing origin":
				child.HasOriginPC = false
			case "child false DUP":
				childDup.stackProduced[1] = parentRef
			case "child cycle":
				child.Initializer[0], childStore.stackConsumed[0] = child, childRef
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.privateDelegationCompletedArray(delegationArrayMaterialization{node: node, array: parent, ref: parentRef}, call, invoke, 0, map[int]*OpCode{11: parentDup}); got != (variant == "original") {
				t.Fatalf("nested ownership=%v", got)
			}
			if call.Arguments[0] != parentRef || parent.Initializer[0] != child || len(node.Next) != 0 {
				t.Fatal("ownership proof mutated the retained source graph")
			}
		})
	}
}
