package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestCarriedPrimitiveJoinRequiresClosedTypedBypass(t *testing.T) {
	for _, kind := range []string{"closed", "reversed condition", "long", "float", "double", "outside entry", "extra join predecessor", "backedge", "branch in arm", "local write", "handler", "protected", "missing root stack", "extra root word", "missing updated stack", "extra updated word", "different category", "effectful seed", "reference seed", "missing simulation", "existing edge copy", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			n := mergeFixture([][]int{{1, 2}, {2}, nil})
			root, arm, merge := n[0], n[1], n[2]
			root.Instr = &Instruction{OpCode: OP_IFNULL}
			arm.Instr = &Instruction{OpCode: OP_IADD}
			merge.Instr = &Instruction{OpCode: OP_IRETURN}
			for _, x := range n {
				x.CurrentOffset++
			}
			typ := types.NewJavaPrimer(types.JavaInteger)
			switch kind {
			case "long":
				typ = types.NewJavaPrimer(types.JavaLong)
			case "float":
				typ = types.NewJavaPrimer(types.JavaFloat)
			case "double":
				typ = types.NewJavaPrimer(types.JavaDouble)
			}
			seed := values.NewJavaLiteral(7, typ)
			result := values.NewJavaLiteral(9, typ)
			root.StackEntry = newStackItem(NewEmptyStackEntry(), seed)
			arm.StackEntry = newStackItem(NewEmptyStackEntry(), result)
			sim := NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{merge: sim}}
			switch kind {
			case "reversed condition":
				root.Target[0], root.Target[1] = root.Target[1], root.Target[0]
			case "outside entry":
				LinkOpcode(&OpCode{CurrentOffset: 0}, arm)
			case "extra join predecessor":
				LinkOpcode(&OpCode{CurrentOffset: 0}, merge)
			case "backedge":
				arm.Target = []*OpCode{root}
			case "branch in arm":
				arm.Target = append(arm.Target, merge)
			case "local write":
				arm.Instr.OpCode = OP_ISTORE_0
			case "handler":
				arm.IsCatch = true
			case "protected":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 9, HandlerPc: 12}}
			case "missing root stack":
				root.StackEntry = nil
			case "extra root word":
				root.StackEntry = newStackItem(root.StackEntry, seed)
			case "missing updated stack":
				arm.StackEntry = nil
			case "extra updated word":
				arm.StackEntry = newStackItem(arm.StackEntry, result)
			case "different category":
				arm.StackEntry.value = values.NewJavaLiteral(9, types.NewJavaPrimer(types.JavaLong))
			case "effectful seed":
				root.StackEntry.value = values.NewFunctionCallExpression(nil, &values.JavaClassMember{Name: "p.Effects", Member: "seed", Description: "()I"}, types.NewJavaFuncType("", nil, typ))
			case "reference seed":
				root.StackEntry.value = values.NewJavaLiteral(nil, types.NewJavaClass("p.Value"))
			case "missing simulation":
				d.opcodeToSimulateStack = nil
			case "existing edge copy":
				if !d.lowerCarriedPrimitiveJoin(merge, root, values.NewSlotValue(seed, typ)) {
					t.Fatal("initial control")
				}
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			beforeRoot, beforeArm := root.StackEntry, arm.StackEntry
			beforeEdges, beforeSnapshots, beforeRefs := len(d.effectfulStackPhiEdges), len(d.carriedPrimitivePhiSeeds), len(d.disFoldRef)
			slot := values.NewSlotValue(seed, typ)
			got := d.lowerCarriedPrimitiveJoin(merge, root, slot)
			want := kind == "closed" || kind == "reversed condition" || kind == "long" || kind == "float" || kind == "double"
			if got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
			if root.StackEntry != beforeRoot || arm.StackEntry != beforeArm {
				t.Fatal("original stack evidence changed")
			}
			if !want {
				if slot.GetValue() != seed || len(d.effectfulStackPhiEdges) != beforeEdges || len(d.carriedPrimitivePhiSeeds) != beforeSnapshots || len(d.disFoldRef) != beforeRefs {
					t.Fatal("rejected proof published partial transformation")
				}
				return
			}
			ref, ok := slot.GetValue().(*values.JavaRef)
			if !ok {
				t.Fatal("missing phi local")
			}
			if d.effectfulStackPhiEdges[root] != nil || d.effectfulStackPhiEdges[arm].LeftValue != ref || d.effectfulStackPhiEdges[arm].JavaValue != result {
				t.Fatal("copy must occur only on updating edge")
			}
			snap := d.carriedPrimitivePhiSeeds[root]
			if snap == nil || snap.LeftValue != ref || snap.JavaValue != seed {
				t.Fatal("bypass initialization must precede condition")
			}
		})
	}
}

func TestCarriedPrimitiveInstructionPacketRequiresItsActualWriter(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	value := values.NewJavaLiteral(7, typ)
	ref := values.NewJavaRef(utils.NewRootVariableId(), value, typ)
	op := &OpCode{Id: 4, Instr: &Instruction{OpCode: OP_IFNULL}}
	writer := statements.NewAssignStatement(ref, value, true)
	d := &Decompiler{carriedPrimitivePhiSeeds: map[*OpCode]*statements.AssignStatement{op: writer}}
	for _, kind := range []string{"closed", "copied writer", "foreign opcode", "swapped", "extra node", "different opcode id", "nonconditional tail"} {
		t.Run(kind, func(t *testing.T) {
			first := NewNode(writer)
			first.Id = op.Id
			last := NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			last.Id = op.Id
			chain := []*Node{first, last}
			instruction := op
			switch kind {
			case "copied writer":
				first.Statement = statements.NewAssignStatement(ref, value, true)
			case "foreign opcode":
				copy := *op
				instruction = &copy
			case "swapped":
				chain[0], chain[1] = chain[1], chain[0]
			case "extra node":
				chain = append([]*Node{NewNode(writer)}, chain...)
			case "different opcode id":
				first.Id++
			case "nonconditional tail":
				last.Statement = writer
			}
			if d.carriedPrimitiveSourceChain(chain, last, instruction) != (kind == "closed") {
				t.Fatal("unowned instruction packet accepted", kind)
			}
		})
	}
}
