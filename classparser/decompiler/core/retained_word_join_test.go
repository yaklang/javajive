package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestRetainedReferenceJoinRequiresOriginalClosedSnapshot(t *testing.T) {
	for _, kind := range []string{"closed", "unsimulated arm", "same result", "outside entry", "bypass", "backedge", "handler", "protected", "extra word", "conditional incoming", "missing producer", "missing root word", "parameter", "this", "mutable alias", "primitive result", "typed nil", "slot budget", "graph budget", "edge budget"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaClass("p.Node")
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.NewJavaLiteral("origin", typ), typ)
			nodes := mergeFixture([][]int{{1, 2}, {3}, {3}, nil})
			root, left, right, merge := nodes[0], nodes[1], nodes[2], nodes[3]
			for _, n := range nodes {
				n.CurrentOffset++
			}
			for _, n := range nodes {
				n.Instr = &Instruction{OpCode: OP_GOTO}
			}
			root.Instr.OpCode = OP_IFNULL
			root.StackEntry = newStackItem(NewEmptyStackEntry(), ref)
			producer := &OpCode{CurrentOffset: 0, Instr: &Instruction{OpCode: OP_DUP}, stackProduced: []values.JavaValue{ref}}
			LinkOpcode(producer, root)
			left.StackEntry = newStackItem(NewEmptyStackEntry(), values.NewJavaLiteral("left", types.NewJavaPrimer(types.JavaString)))
			right.StackEntry = newStackItem(NewEmptyStackEntry(), values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")))
			d := &Decompiler{opcodeIdToRef: map[*OpCode][][2]any{}}
			switch kind {
			case "unsimulated arm":
				right.StackEntry = nil
			case "same result":
				right.StackEntry = left.StackEntry
			case "outside entry":
				LinkOpcode(&OpCode{CurrentOffset: 0}, left)
			case "bypass":
				LinkOpcode(&OpCode{CurrentOffset: 0}, merge)
			case "backedge":
				LinkOpcode(left, root)
			case "handler":
				right.IsCatch = true
			case "protected":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 50, HandlerPc: 60}}
			case "extra word":
				right.StackEntry = newStackItem(right.StackEntry, ref)
			case "conditional incoming":
				LinkOpcode(left, right)
			case "missing producer":
				producer.stackProduced = nil
			case "missing root word":
				root.StackEntry = NewEmptyStackEntry()
			case "parameter":
				ref.IsParam = true
			case "this":
				ref.IsThis = true
			case "mutable alias":
				d.opcodeIdToRef[left] = [][2]any{{ref, false}}
			case "primitive result":
				right.StackEntry = newStackItem(NewEmptyStackEntry(), values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger)))
			case "typed nil":
				right.StackEntry.value = (*values.JavaRef)(nil)
			case "slot budget":
				var slot values.JavaValue = ref
				for i := 0; i < 129; i++ {
					slot = values.NewSlotValue(slot, nil)
				}
				right.StackEntry.value = slot
			case "graph budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "edge budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphEdges: 1})
			}
			rootVal := root.StackEntry
			leftVal, rightVal := left.StackEntry, right.StackEntry
			got := d.retainedReferenceJoinRoot(merge, []*OpCode{root})
			want := kind == "closed" || kind == "same result"
			if (got == root) != want {
				t.Fatalf("accepted=%v want=%v", got == root, want)
			}
			if root.StackEntry != rootVal || left.StackEntry != leftVal || right.StackEntry != rightVal || len(d.effectfulStackPhiEdges) != 0 || len(d.disFoldRef) != 0 {
				t.Fatal("proof mutated original graph")
			}
			if want {
				d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{merge: NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())}
				slot := values.NewSlotValue(left.StackEntry.value, left.StackEntry.value.Type())
				if !d.lowerClosedStackPhi(merge, []*OpCode{root}, slot, false) {
					t.Fatal("certified retained word was mistaken for an unchanged prefix")
				}
				for _, pred := range []*OpCode{left, right} {
					if d.effectfulStackPhiEdges[pred] == nil || d.effectfulStackPhiEdges[pred].JavaValue != pred.StackEntry.value {
						t.Fatal("original incoming edge value lost")
					}
				}
			}
		})
	}
}
