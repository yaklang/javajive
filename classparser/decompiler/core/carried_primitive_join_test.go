package core

import (
	"context"
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
			beforeEdges, beforeSnapshots, beforeRefs := len(d.effectfulStackPhiEdges), len(d.evaluationSnapshots), len(d.disFoldRef)
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
				if slot.GetValue() != seed || len(d.effectfulStackPhiEdges) != beforeEdges || len(d.evaluationSnapshots) != beforeSnapshots || len(d.disFoldRef) != beforeRefs {
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
			snap := d.evaluationSnapshots[root]
			if len(snap) != 1 || snap[0].Ref != ref || snap[0].Value != seed {
				t.Fatal("bypass initialization must precede condition")
			}
		})
	}
}
