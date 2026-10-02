package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestEffectfulStackPhiRequiresClosedForwardRegion(t *testing.T) {
	for _, kind := range []string{"valid", "pure pop", "outside entry", "back edge", "handler", "protected effect", "extra stack word", "conditional predecessor", "missing simulation"} {
		t.Run(kind, func(t *testing.T) {
			merge := &OpCode{CurrentOffset: 40, Instr: &Instruction{OpCode: OP_ARETURN}}
			root := &OpCode{CurrentOffset: 1, Instr: &Instruction{OpCode: OP_IFEQ}}
			pop := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_POP}, Source: []*OpCode{root}}
			left := &OpCode{CurrentOffset: 20, Instr: &Instruction{OpCode: OP_GOTO}, Source: []*OpCode{pop}, Target: []*OpCode{merge}}
			right := &OpCode{CurrentOffset: 30, Instr: &Instruction{OpCode: OP_INVOKESTATIC}, Source: []*OpCode{root}, Target: []*OpCode{merge}}
			root.Target = []*OpCode{pop, right}
			pop.Target = []*OpCode{left}
			merge.Source = []*OpCode{left, right}
			typ := types.NewJavaClass("java.lang.String")
			a := values.NewJavaLiteral("a", types.NewJavaPrimer(types.JavaString))
			b := values.NewFunctionCallExpression(nil, &values.JavaClassMember{Name: "p.Factory", Member: "text", Description: "()Ljava/lang/String;"}, types.NewJavaFuncType("", nil, typ))
			pop.stackConsumed = []values.JavaValue{b}
			left.StackEntry = newStackItem(NewEmptyStackEntry(), a)
			right.StackEntry = newStackItem(NewEmptyStackEntry(), b)
			slot := values.NewSlotValue(a, a.Type())
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{merge: NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())}}
			accepted := kind == "valid"
			switch kind {
			case "pure pop":
				pop.stackConsumed = []values.JavaValue{a}
			case "outside entry":
				pop.Source = append(pop.Source, &OpCode{CurrentOffset: 2})
			case "back edge":
				pop.Target = []*OpCode{root}
			case "handler":
				pop.IsCatch = true
			case "protected effect":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 20}}
			case "extra stack word":
				right.StackEntry = newStackItem(right.StackEntry, b)
			case "conditional predecessor":
				left.Target = append(left.Target, right)
			case "missing simulation":
				d.opcodeToSimulateStack = nil
			}
			if got := d.lowerEffectfulStackPhi(merge, []*OpCode{root}, slot); got != accepted {
				t.Fatalf("lowered=%v want %v", got, accepted)
			}
			if !accepted {
				if slot.GetValue() != a || len(d.effectfulStackPhiEdges) != 0 || len(d.disFoldRef) != 0 {
					t.Fatal("rejected proof changed IR")
				}
				return
			}
			ref, ok := slot.GetValue().(*values.JavaRef)
			if !ok || ref.WebDeclType == nil || len(d.disFoldRef) != 1 || d.disFoldRef[0] != ref {
				t.Fatal("unsolved or foldable phi")
			}
			if d.effectfulStackPhiEdges[left].LeftValue != ref || d.effectfulStackPhiEdges[right].LeftValue != ref || d.effectfulStackPhiEdges[left].JavaValue != a || d.effectfulStackPhiEdges[right].JavaValue != b {
				t.Fatal("incoming edge values changed")
			}
			if len(pop.stackConsumed) != 1 || pop.stackConsumed[0] != b {
				t.Fatal("discarded invocation lost")
			}
		})
	}
}
