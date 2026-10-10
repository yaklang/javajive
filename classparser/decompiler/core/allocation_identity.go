package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// Called during stack simulation, before opcodeToSimulateStack is finalized.
// Use the already simulated constructor invocation and object identity, rather
// than a type-name match or a later-phase PC lookup. Only the linear successful
// constructor continuation contains an initialized object to duplicate.
func (d *Decompiler) isInitializedAllocationAtDup(allocation *values.NewExpression, dup *OpCode) bool {
	if d == nil || allocation == nil || dup == nil || dup.Instr == nil || dup.Instr.OpCode != OP_DUP {
		return false
	}
	var initializer *OpCode
	for op, call := range d.invokeFuncCall {
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESPECIAL || call == nil || call.FunctionName != "<init>" || values.UnpackSoltValue(call.Object) != allocation {
			continue
		}
		if initializer != nil {
			return false
		}
		initializer = op
	}
	return initializer != nil && initializer.CurrentOffset < dup.CurrentOffset && branchArraySinglePath(d, initializer, dup)
}
