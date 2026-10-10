package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBranchArrayLeafRequiresPrivateCompletedInitializer(t *testing.T) {
	for _, change := range []string{"valid", "alternate entry", "handler boundary", "partial fill", "escape", "stored as element", "intervening call", "allocation outside arm", "different arm value"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
			array := values.NewNewArrayExpression(typ, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
			array.OriginPC, array.HasOriginPC = 10, true
			array.EvaluationEndPC, array.HasEvaluationEndPC = 14, true
			array.Initializer = []values.JavaValue{values.JavaNull}
			ref := values.NewJavaRef(utils.NewRootVariableId(), array, typ)
			alloc := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 10}
			dup := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 13}
			store := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 14, stackConsumed: []values.JavaValue{values.JavaNull, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), ref}}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 15}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_ARETURN}, CurrentOffset: 20}
			ops := []*OpCode{alloc, dup, store, jump, merge}
			d := &Decompiler{opCodes: ops, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}}
			for i, op := range ops {
				d.opcodeToSimulateStack[op] = nil
				if i > 0 {
					op.Source = []*OpCode{ops[i-1]}
					ops[i-1].Target = []*OpCode{op}
				}
			}
			c := branchArrayLeaf{ref, array, values.NewSlotValue(ref, nil), alloc, jump, merge}
			switch change {
			case "alternate entry":
				store.Source = append(store.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 20, HandlerPc: 30}}
			case "partial fill":
				array.HasEvaluationEndPC = false
			case "escape":
				merge.stackConsumed = []values.JavaValue{ref}
			case "stored as element":
				store.stackConsumed[0] = ref
			case "intervening call":
				jump.Instr = &Instruction{OpCode: OP_INVOKESTATIC}
			case "allocation outside arm":
				c.entry = dup
			case "different arm value":
				c.slot.ResetValue(values.JavaNull)
			}
			if got := d.branchArrayLeafIsolated(c); got != (change == "valid") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
