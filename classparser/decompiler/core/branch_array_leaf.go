package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type branchArrayLeaf struct {
	ref                *values.JavaRef
	array              *values.NewExpression
	slot               *values.SlotValue
	entry, leaf, merge *OpCode
}

// A stack-only array literal is one value on a conditional arm. Its DUP
// temporary is an implementation detail, but cannot be discarded until all
// initializer stores have been folded. Ordinary variable folding cannot move
// an allocation across a merge, and retaining just the temporary in a ternary
// loses its branch-local definition when the branch is dissolved.
//
// This proof retains the allocation and its elements in the SAME selected arm:
// no alternate entries, no handler changes, no partial fill, no use that could
// expose identity, and no instruction evaluated after the completed initializer
// except its routing jump. It applies to primitive and reference arrays alike.
func (d *Decompiler) branchArrayLeafIsolated(c branchArrayLeaf) bool {
	if c.ref == nil || c.array == nil || c.slot == nil || c.entry == nil || c.leaf == nil || c.merge == nil ||
		!c.array.HasOriginPC || !c.array.HasEvaluationEndPC || len(c.array.Initializer) == 0 ||
		values.UnpackSoltValue(c.slot) != c.ref {
		return false
	}
	allocation, end := d.opcodeAtOffset(c.array.OriginPC), d.opcodeAtOffset(c.array.EvaluationEndPC)
	if allocation == nil || end == nil || allocation.Instr == nil ||
		(allocation.Instr.OpCode != OP_ANEWARRAY && allocation.Instr.OpCode != OP_NEWARRAY) ||
		!branchArraySinglePath(d, c.entry, c.leaf) ||
		len(c.leaf.Target) != 1 || c.leaf.Target[0] != c.merge ||
		!sameHandlerCoverage(d.handlersAt(c.leaf), d.handlersAt(c.merge)) {
		return false
	}
	seenAllocation, seenEnd := false, false
	for op := c.entry; ; op = op.Target[0] {
		if op == allocation {
			seenAllocation = true
		}
		if op == end {
			seenEnd = seenAllocation
		} else if seenEnd {
			if op.Instr == nil || (op.Instr.OpCode != OP_GOTO && op.Instr.OpCode != OP_GOTO_W && op.Instr.OpCode != OP_NOP) {
				return false
			}
		}
		if op == c.leaf {
			break
		}
	}
	if !seenEnd {
		return false
	}
	stores := 0
	for _, op := range d.opCodes {
		for index, value := range op.stackConsumed {
			if values.UnpackSoltValue(value) != c.ref {
				continue
			}
			if op.Instr == nil || int(op.CurrentOffset) <= c.array.OriginPC || int(op.CurrentOffset) > c.array.EvaluationEndPC {
				return false
			}
			switch op.Instr.OpCode {
			case OP_DUP:
			case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
				if index != 2 {
					return false
				}
				stores++
			default:
				return false
			}
		}
	}
	return stores == len(c.array.Initializer)
}

func (d *Decompiler) inlineBranchArrayLeaves() {
	for _, c := range d.branchArrayLeaves {
		if !d.branchArrayLeafIsolated(c) {
			continue
		}
		var definition *Node
		count := 0
		WalkGraph[*Node](d.RootNode, func(n *Node) ([]*Node, error) {
			if assign, ok := n.Statement.(*statements.AssignStatement); ok && assign.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef); ok && values.SameLocal(ref, c.ref) {
					count++
					if values.UnpackSoltValue(assign.JavaValue) == c.array {
						definition = n
					}
				}
			}
			return n.Next, nil
		})
		if count != 1 || definition == nil || len(definition.Next) != 1 || definition == d.RootNode {
			continue
		}
		next := definition.Next[0]
		for _, source := range append([]*Node(nil), definition.Source...) {
			source.ReplaceNextSliceKeepOrder(definition, []*Node{next})
			source.ReplaceSwitchTarget(definition, next)
			if source.JmpNode == definition {
				source.JmpNode = next
			}
		}
		definition.RemoveAllSource()
		definition.RemoveAllNext()
		c.slot.ResetValue(c.array)
	}
}

// Fold the innermost completed array into its single parent element store.
// Repeating this bottom-up lets RewriteNewArrayList see contiguous outer
// stores, without ever publishing an incomplete array or copying an alias.
func (d *Decompiler) inlineNestedArrayInitializers(origins map[int]*OpCode) bool {
	changed := false
	WalkGraph[*Node](d.RootNode, func(n *Node) ([]*Node, error) {
		if n == d.RootNode || len(n.Next) != 1 {
			return n.Next, nil
		}
		a, ok := n.Statement.(*statements.AssignStatement)
		if !ok || a.ArrayMember != nil {
			return n.Next, nil
		}
		ref, ok := values.UnpackSoltValue(a.LeftValue).(*values.JavaRef)
		if !ok || ref == nil || ref.IsParam || ref.IsThis {
			return n.Next, nil
		}
		array, ok := values.UnpackSoltValue(a.JavaValue).(*values.NewExpression)
		if !ok || !array.IsArray() || !array.HasOriginPC || !array.HasEvaluationEndPC || len(array.Initializer) == 0 {
			return n.Next, nil
		}
		target := n.Next[0]
		store, ok := target.Statement.(*statements.AssignStatement)
		if !ok || store.ArrayMember == nil || values.UnpackSoltValue(store.JavaValue) != ref {
			return n.Next, nil
		}
		allocation, sink := d.opcodeAtOffset(array.OriginPC), origins[target.Id]
		if allocation == nil || sink == nil || !branchArraySinglePath(d, allocation, sink) || !d.canInlineValue(array, n, target, origins, ref, sink) {
			return n.Next, nil
		}
		uses := 0
		for _, op := range d.opCodes {
			for index, value := range op.stackConsumed {
				if values.UnpackSoltValue(value) != ref {
					continue
				}
				if op == sink && index == 0 {
					uses++
					continue
				}
				if int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC && op.Instr != nil {
					if op.Instr.OpCode == OP_DUP || (index == 2 && isArrayElementStore(op.Instr.OpCode)) {
						continue
					}
				}
				return n.Next, nil
			}
		}
		if uses != 1 {
			return n.Next, nil
		}
		for _, pred := range append([]*Node(nil), n.Source...) {
			pred.ReplaceNextSliceKeepOrder(n, []*Node{target})
			if pred.JmpNode == n {
				pred.JmpNode = target
			}
		}
		n.RemoveAllSource()
		n.RemoveAllNext()
		store.JavaValue = array
		changed = true
		return []*Node{target}, nil
	})
	return changed
}

func isArrayElementStore(op int) bool {
	switch op {
	case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
		return true
	}
	return false
}
