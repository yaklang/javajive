package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestArrayCopyCheckcastMergeProof(t *testing.T) {
	for _, kind := range []string{"exact array", "null", "wrong component", "wrong dimension", "object", "missing stack", "local store", "different handler", "shared arm", "backward merge"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaDouble))
			input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			cast := values.NewCastExpression(input, typ, 10).(*values.CastExpression)
			ref := values.NewJavaRef(utils.NewRootVariableId(), cast, typ)
			branch := &OpCode{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 1}
			start := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 4}
			producer := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 10, stackProduced: []values.JavaValue{ref}}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 13, StackEntry: newStackItem(nil, values.NewSlotValue(ref, typ))}
			other := &OpCode{Instr: &Instruction{OpCode: OP_GETFIELD}, CurrentOffset: 16, StackEntry: newStackItem(nil, values.NewJavaRef(utils.NewRootVariableId(), nil, typ))}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_PUTFIELD}, CurrentOffset: 20, stackConsumed: []values.JavaValue{input, ref}}
			branch.Target = []*OpCode{start, other}
			start.Source = []*OpCode{branch}
			start.Target = []*OpCode{producer}
			producer.Source = []*OpCode{start}
			producer.Target = []*OpCode{jump}
			jump.Source = []*OpCode{producer}
			jump.Target = []*OpCode{merge}
			other.Source = []*OpCode{branch}
			other.Target = []*OpCode{merge}
			merge.Source = []*OpCode{jump, other}
			source := NewNode(statements.NewAssignStatement(ref, cast, true))
			target := NewNode(statements.NewGOTOStatement())
			source.Id, target.Id = 1, 2
			source.AddNext(target)
			origins := map[int]*OpCode{1: producer, 2: jump}
			d := &Decompiler{opCodes: []*OpCode{branch, start, producer, jump, other, merge}, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, jump: nil}}
			switch kind {
			case "null":
				other.StackEntry = newStackItem(nil, values.JavaNull)
			case "wrong component":
				other.StackEntry = newStackItem(nil, values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaLong))))
			case "wrong dimension":
				other.StackEntry = newStackItem(nil, values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(typ)))
			case "object":
				other.StackEntry = newStackItem(nil, input)
			case "missing stack":
				other.StackEntry = nil
			case "local store":
				merge.Instr.OpCode = OP_ASTORE_1
			case "different handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 13, HandlerPc: 30}}
			case "shared arm":
				other.Source = append(other.Source, &OpCode{Instr: &Instruction{OpCode: OP_NOP}})
			case "backward merge":
				merge.CurrentOffset = 8
			}
			got := d.canInlineCheckcastIntoBranchMerge(cast, source, target, origins, ref)
			if got != (kind == "exact array" || kind == "null") {
				t.Fatalf("proof accepted=%v", got)
			}
		})
	}
}

func TestExactArrayTypeUsesComponentIdentity(t *testing.T) {
	array := func(name string) types.JavaType { return types.NewJavaArrayType(types.NewJavaClass(name)) }
	if !sameExactArrayType(array("pkg/Value"), array("pkg.Value")) || sameExactArrayType(array("one.Value"), array("two.Value")) || sameExactArrayType(array("pkg.Value"), types.NewJavaArrayType(array("pkg.Value"))) || sameExactArrayType(nil, array("pkg.Value")) {
		t.Fatal("array identity proof ignored component or dimension")
	}
}
