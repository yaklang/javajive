package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A uniquely reached immediate zero-branch can consume the three-way result as
// a predicate. Every other consumer must receive the original int word, never
// a Boolean approximation. NOPs are only a readability optimization; a long or
// cyclic prefix conservatively keeps the fully materialized numeric result.
func comparisonHasDirectZeroBranch(op *OpCode) bool {
	if op == nil {
		return false
	}
	prev := op
	for steps := 0; steps < 32; steps++ {
		if len(prev.Target) != 1 {
			return false
		}
		next := prev.Target[0]
		if next == nil || next.Instr == nil || len(next.Source) != 1 || next.Source[0] != prev || next.CurrentOffset <= prev.CurrentOffset {
			return false
		}
		switch next.Instr.OpCode {
		case OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE:
			return true
		case OP_NOP:
			prev = next
		default:
			return false
		}
	}
	return false
}

// Three-way source expressions test a captured word twice. Materialize both
// inputs, left before right, at the original comparison position and retain
// their declarations against folding. This preserves volatile reads, calls,
// exceptions, local reassignments and the -1/0/+1 result category.
func (d *Decompiler) comparisonWordOperands(sim StackSimulation, op *OpCode, left, right values.JavaValue) (values.JavaValue, values.JavaValue) {
	if d.comparisonWordInputs == nil {
		d.comparisonWordInputs = map[*OpCode][]*statements.AssignStatement{}
	}
	inputs := make([]*statements.AssignStatement, 0, 2)
	refs := make([]values.JavaValue, 0, 2)
	for _, value := range []values.JavaValue{left, right} {
		ref := sim.NewVar(value)
		ref.MarkOriginalStackMaterialization(int(op.CurrentOffset), op.Instr.OpCode, value)
		d.disFoldRef = append(d.disFoldRef, ref)
		inputs = append(inputs, statements.NewAssignStatement(ref, value, true))
		refs = append(refs, ref)
	}
	d.comparisonWordInputs[op] = inputs
	return refs[0], refs[1]
}
