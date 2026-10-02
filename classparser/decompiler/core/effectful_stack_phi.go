package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
)

// A conditional value with a discarded invocation or field store in an arm is a sequence,
// not a Java conditional expression. Keep the original statement CFG and
// materialize the value on each incoming edge. The bounded proof below accepts
// only a closed, forward region with a single stack word at its exit; handler
// joins and joins with values below that word need separate lowering.
func (d *Decompiler) lowerEffectfulStackPhi(merge *OpCode, conditions []*OpCode, slot *values.SlotValue) bool {
	return d.lowerClosedStackPhi(merge, conditions, slot, true)
}

// Expression planning may also decline to own a branch-local cast producer.
// Preserve that definition on its original edge instead of emitting a ternary
// which reads the producer after its selected branch has disappeared.
func (d *Decompiler) lowerClosedStackPhi(merge *OpCode, conditions []*OpCode, slot *values.SlotValue, requireEffect bool) bool {
	if merge == nil || slot == nil || len(conditions) == 0 || len(merge.Source) < 2 {
		return false
	}
	root := conditions[0]
	for _, n := range conditions {
		if n.CurrentOffset < root.CurrentOffset {
			root = n
		}
	}
	region := map[*OpCode]bool{}
	pending := []*OpCode{root}
	effect := false
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n == merge || region[n] {
			continue
		}
		if n == nil || n.Instr == nil || n.CurrentOffset < root.CurrentOffset || n.CurrentOffset >= merge.CurrentOffset ||
			n.IsCatch || n.IsTryCatchParent || len(n.Target) == 0 || len(region) >= 512 {
			return false
		}
		region[n] = true
		for _, entry := range d.ExceptionTable {
			if n.CurrentOffset >= entry.StartPc && n.CurrentOffset < entry.EndPc {
				return false
			}
		}
		for _, next := range n.Target {
			if next == nil || next.CurrentOffset <= n.CurrentOffset {
				return false
			}
			pending = append(pending, next)
		}
		if n.Instr.OpCode == OP_POP || n.Instr.OpCode == OP_POP2 {
			for _, v := range n.stackConsumed {
				if keepDiscardedStackValue(v) {
					effect = true
				}
			}
		}
		if n.Instr.OpCode == OP_PUTFIELD || n.Instr.OpCode == OP_PUTSTATIC {
			// Keep the field store on its original selected arm, before its
			// retained stack value is cast or consumed. Reconstructing only
			// the value can hoist the store or lose its producer entirely.
			effect = true
		}
	}
	if (requireEffect && !effect) || merge.IsCatch || merge.IsTryCatchParent {
		return false
	}
	for n := range region {
		if n == root {
			continue
		}
		for _, pred := range n.Source {
			if !region[pred] {
				return false
			}
		}
	}
	var typ types.JavaType
	var provider types.SuperTypeProvider
	if d.FunctionContext != nil {
		provider = d.FunctionContext.SiblingSuperTypes
	}
	for _, pred := range merge.Source {
		if !region[pred] || len(pred.Target) != 1 || pred.Target[0] != merge || pred.StackEntry == nil ||
			pred.StackEntry.depth != 1 || pred.StackEntry.value == nil || pred.StackEntry.value.Type() == nil {
			return false
		}
		v := pred.StackEntry.value
		incomingType := v.Type()
		if primitive, ok := incomingType.RawType().(*types.JavaPrimer); ok && primitive.Name == types.JavaString {
			incomingType = types.NewJavaClass("java.lang.String")
		}
		if values.IsNullLiteral(v) {
			continue
		}
		if typ == nil {
			typ = incomingType
		} else if !reflect.DeepEqual(typ.RawType(), incomingType.RawType()) {
			typ = types.BridgedCommonSuperType(typ, incomingType, provider)
		}
		if typ == nil {
			return false
		}
	}
	if typ == nil {
		return false
	}
	sim := d.opcodeToSimulateStack[merge]
	if sim == nil {
		return false
	}
	ref := sim.NewVar(values.NewJavaLiteral(nil, typ))
	ref.ResetVarType(typ)
	ref.WebDeclType = typ.Copy()
	slot.TmpType = typ.Copy()
	if d.effectfulStackPhiEdges == nil {
		d.effectfulStackPhiEdges = map[*OpCode]*statements.AssignStatement{}
	}
	for _, pred := range merge.Source {
		d.effectfulStackPhiEdges[pred] = statements.NewAssignStatement(ref, pred.StackEntry.value, false)
		// The new edge assignment is a real use which did not exist during
		// stack simulation. Keep its producer local: an old single-use fold
		// callback otherwise rewrites the original merged slot and deletes
		// the CHECKCAST while this edge still reads its uncast input.
		if incoming, ok := values.UnpackSoltValue(pred.StackEntry.value).(*values.JavaRef); ok && incoming != nil {
			d.disFoldRef = append(d.disFoldRef, incoming)
		}
	}
	d.disFoldRef = append(d.disFoldRef, ref)
	// These producers have new edge uses. Their pre-lowering fold callbacks
	// refer to the old merged stack slot and cannot authorize their removal.
	for op := range region {
		if op.Instr.OpCode == OP_CHECKCAST {
			for _, value := range op.stackProduced {
				if producer, ok := values.UnpackSoltValue(value).(*values.JavaRef); ok && producer != nil {
					d.disFoldRef = append(d.disFoldRef, producer)
				}
			}
		}
	}
	slot.ResetValue(ref)
	return true
}
