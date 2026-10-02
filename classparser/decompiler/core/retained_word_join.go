package core

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A fork can keep a duplicated reference on the stack while each arm consumes
// it and produces a different result. Equal fork/join heights therefore do not
// prove value identity. Keep this closed, single-word join as statement-owned
// edge copies; stores and nested value diamonds must stay on their original arm.
func (d *Decompiler) retainedReferenceJoinRoot(merge *OpCode, candidates []*OpCode) *OpCode {
	root := d.retainedReferenceRoutingRoot(merge, candidates)
	if root == nil {
		return nil
	}
	if merge == nil || len(merge.Source) < 2 || len(merge.Source) > 512 {
		return nil
	}
	for _, pred := range merge.Source {
		if d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return nil
		}
		if pred == nil || len(pred.Target) != 1 || pred.Target[0] != merge || pred.StackEntry == nil || pred.StackEntry.depth != 1 {
			return nil
		}
		v, ok := retainedJoinValue(pred.StackEntry.value)
		if !ok {
			return nil
		}
		if typ := v.Type(); typ == nil {
			return nil
		} else {
			switch raw := typ.RawType().(type) {
			case *types.JavaClass, *types.JavaArrayType, *types.JavaParameterizedType:
			case *types.JavaPrimer:
				if raw.Name != types.JavaString {
					return nil
				}
			default:
				return nil
			}
		}
	}
	return root
}

// Routing is proved while simulating the first incoming edge, before DFS has
// visited all arms. Actual incoming values are checked again after simulation;
// the early placeholder must never silently select that first edge as a result.
func (d *Decompiler) retainedReferenceRoutingRoot(merge *OpCode, candidates []*OpCode) *OpCode {
	if merge == nil || len(merge.Source) < 2 || len(merge.Source) > 512 {
		return nil
	}
	for _, pred := range merge.Source {
		if pred == nil || len(pred.Target) != 1 || pred.Target[0] != merge {
			return nil
		}
	}
	root := d.closedNestedValueRoot(merge, candidates)
	if root == nil || root.StackEntry == nil || root.StackEntry.depth != 1 || len(d.handlersAt(root)) != 0 {
		return nil
	}
	v, ok := retainedJoinValue(root.StackEntry.value)
	if !ok {
		return nil
	}
	ref, ok := v.(*values.JavaRef)
	if !ok || ref == nil || ref.IsParam || ref.IsThis || ref.Val == nil || ref.Id == nil {
		return nil
	}
	// The snapshot must have an original producer on the unique prefix into
	// this fork. A source initializer or matching name is not that evidence.
	producer := false
	for n, steps := root, 0; steps < 512; steps++ {
		if d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || len(n.Source) != 1 {
			return nil
		}
		prev := n.Source[0]
		if prev == nil || prev.CurrentOffset >= n.CurrentOffset {
			return nil
		}
		if d.opcodeProducesLocal(prev, ref) {
			producer = true
			break
		}
		n = prev
	}
	if !producer {
		return nil
	}
	// A retained stack snapshot must not alias a mutable local store in its
	// region. DUP's private producer remains before the fork, not re-evaluated
	// on whichever return edge happens to be selected.
	seen := map[*OpCode]bool{}
	pending := []*OpCode{root}
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n == merge || seen[n] {
			continue
		}
		if d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || d.Work.Charge(workbudget.CounterGraphEdges, int64(len(n.Target))) != nil {
			return nil
		}
		seen[n] = true
		for _, row := range d.opcodeIdToRef[n] {
			if stored, ok := row[0].(*values.JavaRef); ok && stored != nil && (stored == ref || stored.VarUid == ref.VarUid) {
				return nil
			}
		}
		pending = append(pending, n.Target...)
	}
	return root
}
func retainedJoinValue(v values.JavaValue) (values.JavaValue, bool) {
	seen := map[values.JavaValue]bool{}
	for {
		if v == nil || reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() || seen[v] || len(seen) >= 128 {
			return nil, false
		}
		seen[v] = true
		if slot, ok := v.(*values.SlotValue); ok {
			v = slot.GetValue()
			continue
		}
		return v, true
	}
}
