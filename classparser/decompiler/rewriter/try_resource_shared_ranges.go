package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A shared handler is complete here only when its additional original ranges
// enter the exact nested handlers collected into this source body. Metadata
// from a different sibling region must not authorize normal-tail factoring.
// The effect/exit proof separately checks every nested arm before any mutation.
func resourceSharedNestedHandlerRanges(region *core.Node, tr *statements.TryCatchStatement) bool {
	if region == nil || tr == nil || !region.HasProtectedRange || len(tr.Handlers) != 1 || len(region.SharedProtectedRanges) < 2 || len(region.SharedProtectedRanges) > 32 {
		return false
	}
	var rows [][2]int
	for _, row := range region.SharedProtectedRanges {
		if int(row.HandlerPc) != tr.Handlers[0].EntryPC {
			return false
		}
		rows = append(rows, [2]int{int(row.StartPc), int(row.EndPc)})
	}
	ranges, valid := canonicalHandlerRanges(rows)
	original, known := canonicalHandlerRanges(tr.Handlers[0].ProtectedRanges)
	if !valid || !known || !slices.Equal(ranges, original) || len(ranges) < 2 {
		return false
	}
	entries := map[int]bool{region.ProtectedStartPC: true}
	abruptEnds := map[int]int{}
	queue := slices.Clone(tr.TryBody)
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 && len(seen) < 128 {
		st := queue[0]
		queue = queue[1:]
		if st == nil || seen[st] {
			return false
		}
		seen[st] = true
		switch x := st.(type) {
		case *statements.TryCatchStatement:
			if x == nil || len(x.Exception) != len(x.Handlers) || len(x.Exception) != len(x.CatchBodies) {
				return false
			}
			for _, handler := range x.Handlers {
				entries[handler.EntryPC] = true
			}
			queue = append(queue, x.TryBody...)
			for _, body := range x.CatchBodies {
				queue = append(queue, body...)
			}
		case *statements.IfStatement:
			if x == nil {
				return false
			}
			queue = append(queue, x.IfBody...)
			queue = append(queue, x.ElseBody...)
		case *statements.CustomStatement:
			operand, sealed := x.SourceThrowOperand()
			if sealed && x.HasOriginPC && finallyPureLocal(operand) {
				abruptEnds[x.OriginPC+1] = x.OriginPC
			}
		}
	}
	if len(queue) != 0 {
		return false
	}
	for _, interval := range ranges {
		// javac can exclude a normal close between two intervals, then protect
		// only the original local rethrow. Its value has no folded throwing
		// effects; the exact ATHROW endpoint seals this additional abrupt arm.
		throwPC, abrupt := abruptEnds[interval[1]]
		if !entries[interval[0]] && !(abrupt && throwPC >= interval[0]) {
			return false
		}
	}
	return slices.Contains(ranges, [2]int{region.ProtectedStartPC, region.ProtectedEndPC})
}
