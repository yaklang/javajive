package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestDelegationTernaryCompletionEvidence(t *testing.T) {
	for _, scenario := range []string{"registered", "unregistered", "different-value", "equal-boundary", "after-boundary", "different-opcode-identity", "branch-after-merge", "alternate-entry", "cycle", "handler-change"} {
		t.Run(scenario, func(t *testing.T) {
			fresh := func() *values.TernaryExpression {
				return &values.TernaryExpression{TrueValue: &values.JavaClassMember{Name: "sample.Owner", Member: "FIELD"}}
			}
			value := fresh()
			merge, between, array := op(OP_ALOAD_1, 10), op(OP_NOP, 11), op(OP_ANEWARRAY, 12)
			merge.Target, between.Source = []*OpCode{between}, []*OpCode{merge}
			between.Target, array.Source = []*OpCode{array}, []*OpCode{between}
			d := &Decompiler{opCodes: []*OpCode{merge, between, array}, valueTernaryMerges: map[*values.TernaryExpression]*OpCode{value: merge}, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{merge: nil, between: nil, array: nil}}
			want := false
			switch scenario {
			case "registered":
				want = true
			case "unregistered":
				d.valueTernaryMerges = nil
			case "different-value":
				value = fresh()
			case "equal-boundary":
				array.CurrentOffset = 10
			case "after-boundary":
				array.CurrentOffset = 9
			case "different-opcode-identity":
				d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{op(OP_ALOAD_1, 10): nil, between: nil, array: nil}
			case "branch-after-merge":
				between.Target = append(between.Target, op(OP_RETURN, 13))
			case "alternate-entry":
				array.Source = append(array.Source, op(OP_NOP, 9))
			case "cycle":
				between.Target = []*OpCode{merge}
			case "handler-change":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 12, HandlerPc: 20}}
			}
			if got := d.delegationTernaryCompletedBefore(value, array); got != want {
				t.Fatalf("got %v want %v", got, want)
			}
			// This evidence never enables generic expression motion.
			if d.evaluationCompletedBefore(value, array) {
				t.Fatal("unconditional evaluation proof accepted a ternary")
			}
		})
	}
}
