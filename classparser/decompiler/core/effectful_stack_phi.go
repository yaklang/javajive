package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
)

// A conditional value with a discarded invocation in an arm is a sequence,
// not a Java conditional expression. Keep the original statement CFG and
// materialize the value on each incoming edge. The bounded proof below accepts
// only a closed, forward region with a single stack word at its exit; handler
// joins and joins with values below that word need separate lowering.
func (d *Decompiler) lowerEffectfulStackPhi(merge *OpCode, conditions []*OpCode, slot *values.SlotValue) bool {
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
	}
	if !effect || merge.IsCatch || merge.IsTryCatchParent {
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
	}
	d.disFoldRef = append(d.disFoldRef, ref)
	slot.ResetValue(ref)
	return true
}
