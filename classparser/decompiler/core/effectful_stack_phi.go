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
// only a closed, forward region with one selected word above an unchanged stack
// prefix. Handler joins and changing prefixes require separate lowering.
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
		if stackLifetimeWrite(n.Instr.OpCode) && !d.isInlineArrayInitStore(n) {
			// Keep every local/field/array write on its selected arm, before its
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
	// A Boolean consumer can constrain JVM int-category constant arms. Copy
	// only literal 0/1 into that source type; arbitrary ints remain unproved.
	incomingValues := map[*OpCode]values.JavaValue{}
	booleanTarget := false
	if target := slot.Type(); target != nil {
		if primitive, ok := target.RawType().(*types.JavaPrimer); ok {
			booleanTarget = primitive.Name == types.JavaBoolean
		}
	}
	var typ types.JavaType
	var provider types.SuperTypeProvider
	if d.FunctionContext != nil {
		provider = d.FunctionContext.SiblingSuperTypes
	}
	prefix := root.StackEntry
	// In the retained-reference case the fork's one word is consumed by
	// each arm and replaced by the selected result. It is not an unchanged
	// prefix. Only the original closed snapshot certificate permits this
	// interpretation; equal fork/join heights alone are insufficient.
	if prefix != nil && prefix.depth == 1 && d.retainedReferenceJoinRoot(merge, []*OpCode{root}) == root {
		prefix = prefix.parent
	}
	for _, pred := range merge.Source {
		if !region[pred] || len(pred.Target) != 1 || pred.Target[0] != merge || pred.StackEntry == nil ||
			!d.sameStackLifetimePrefix(prefix, pred.StackEntry.parent) || pred.StackEntry.value == nil || pred.StackEntry.value.Type() == nil {
			return false
		}
		v := pred.StackEntry.value
		if prefix != nil && prefix.parent != nil {
			// The unchanged-prefix extension owns primitive words and their
			// lifetime copies. Reference joins above older operands need a
			// separate alias/allocation and nested routing certificate; value
			// identity alone cannot authorize that statement transformation.
			primitive, ok := v.Type().RawType().(*types.JavaPrimer)
			if !ok || primitive.Name == types.JavaString || primitive.Name == types.JavaVoid {
				return false
			}
		}
		if booleanTarget {
			actual, primitive := v.Type().RawType().(*types.JavaPrimer)
			if !primitive || (actual.Name != types.JavaInteger && actual.Name != types.JavaBoolean) {
				return false
			}
			literal, isLiteral := values.UnpackSoltValue(v).(*values.JavaLiteral)
			if actual.Name == types.JavaInteger && !isLiteral {
				return false
			}
			if isLiteral {
				switch data := literal.Data.(type) {
				case int:
					if data != 0 && data != 1 {
						return false
					}
					v = values.NewJavaLiteral(data != 0, types.NewJavaPrimer(types.JavaBoolean))
				case bool:
					if actual.Name != types.JavaBoolean {
						return false
					}
				default:
					return false
				}
			}
		}
		incomingValues[pred] = v
		incomingType, valid := d.closedStackPhiValueType(v, provider)
		if !valid {
			return false
		}
		if incomingType == nil {
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
		assign := statements.NewAssignStatement(ref, incomingValues[pred], false)
		// This is the original edge placement, not a decoded ASTORE.
		// The outgoing stack value is written after that routing instruction.
		assign.OriginPC, assign.HasOriginPC = int(pred.CurrentOffset), true
		d.effectfulStackPhiEdges[pred] = assign
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

// The selected top word may have older operands below it. Those operands must
// be exactly the original prefix at the branch, rather than a second phi. A
// lifetime copy is the same already evaluated word; equal source text or equal
// local names do not prove equality. Neither stack is changed by this proof.
func (d *Decompiler) sameStackLifetimePrefix(first, second *StackItem) bool {
	for steps := 0; steps <= 512; steps++ {
		firstEmpty := first == nil || first.parent == nil
		secondEmpty := second == nil || second.parent == nil
		if firstEmpty || secondEmpty {
			return firstEmpty && secondEmpty
		}
		a, b := first.value, second.value
		if saved := d.stackLifetimeCopies[a]; saved != nil {
			a = saved
		}
		if saved := d.stackLifetimeCopies[b]; saved != nil {
			b = saved
		}
		if a == nil || b == nil || a != b {
			return false
		}
		first, second = first.parent, second.parent
	}
	return false
}

// This recognizes a value-planning candidate, not permission to remove stores.
// The later private array fill/ownership proof must certify all uses, element
// order and handler coverage before any initializer statements disappear.
func (d *Decompiler) isInlineArrayInitStore(cur *OpCode) bool {
	if d.getenv("JDEC_ARRAYINIT_TERNARY_OFF") != "" {
		return false
	}
	switch cur.Instr.OpCode {
	case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
	default:
		return false
	}
	if len(cur.stackConsumed) < 3 {
		return false
	}
	ref := cur.stackConsumed[2]
	if ref == nil {
		return false
	}
	if _, ok := UnpackSoltValue(ref).(*values.NewExpression); ok {
		return true
	}
	if _, ok := GetRealValue(ref).(*values.NewExpression); ok {
		return true
	}
	return false
}
