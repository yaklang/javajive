package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Heap/effect commutation is not enough for a lambda body. javac discovers
// captured variables lexically; moving a producer past another captured read
// can change the physical implementation's argument bindings. Retain the real
// producer when that movement would invert capture discovery. This creates no
// dummy reads, aliases or replacement values.
func (d *Decompiler) lambdaCaptureInliningOrderClosed(value values.JavaValue, source, target *Node, ref *values.JavaRef) bool {
	if len(d.LambdaCaptureOrderSlots) == 0 {
		return true
	}
	if len(d.LambdaCaptureOrderSlots) > 255 || d.Work != nil && d.Work.CheckAlloc(int64(len(d.LambdaCaptureOrderSlots))*64+128) != nil {
		return false
	}
	ranks := make(map[int]int, len(d.LambdaCaptureOrderSlots))
	startWord := 0
	if d.FunctionContext != nil && !d.FunctionContext.IsStatic {
		startWord = 1
	}
	for rank, slot := range d.LambdaCaptureOrderSlots {
		if slot < 0 || slot > 254 || rank == 0 && slot != startWord || rank > 0 && slot <= d.LambdaCaptureOrderSlots[rank-1] {
			return false
		}
		ranks[slot] = rank
	}
	readRanks := func(access values.Access) (int, int, bool) {
		first, last := len(ranks), -1
		if access.Effects&values.EffectOpaque != 0 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(access.Reads))+1) != nil {
			return first, last, false
		}
		for operand := range access.Reads {
			if operand == nil {
				return first, last, false
			}
			slot, known := operand.OriginalParameterSlot()
			if !known {
				if operand.IsParam && !operand.IsThis {
					return first, last, false
				}
				continue
			}
			rank, captured := ranks[slot]
			if captured {
				if rank < first {
					first = rank
				}
				if rank > last {
					last = rank
				}
			}
		}
		return first, last, true
	}
	first, last, known := readRanks(values.InspectAccess(value))
	if !known || last < 0 {
		return known
	}
	if source == nil || target == nil || source == target || ref == nil {
		return false
	}
	// Lexical reads still count when a bytecode/effect proof regards them as
	// already evaluated. Query the same source grammar without that exemption.
	prefix, known := inlineUsePrefix(nil, target.Statement, ref, nil)
	_, prefixLast, closed := readRanks(prefix)
	if !known || !closed || first < prefixLast {
		return false
	}
	seen := map[*Node]bool{}
	for cursor := source; cursor != target; {
		if cursor == nil || seen[cursor] || len(cursor.Next) != 1 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return false
		}
		seen[cursor] = true
		cursor = cursor.Next[0]
		if cursor == nil || hasDistinctPredecessors(cursor) {
			return false
		}
		if cursor != target {
			_, last, known := readRanks(statementAccess(cursor.Statement))
			if !known || first < last {
				return false
			}
		}
	}
	return true
}
