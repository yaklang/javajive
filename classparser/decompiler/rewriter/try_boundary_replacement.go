package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A protected exclusive-end pointer is a semantic reference, even when its
// original GOTO was detached from the normal CFG. Replacing a condition must
// keep that reference on the current structured continuation. Do not infer a
// boundary from node IDs or the first uncovered instruction: a folded input
// may still throw inside the protected region. The complete replacement must
// stay outside every original interval of the affected handlers.
func retargetProtectedIfBoundary(regions []*core.Node, original, replacement *core.Node) {
	if original == nil || replacement == nil {
		return
	}
	if _, ok := replacement.Statement.(*statements.IfStatement); !ok {
		return
	}
	for _, region := range regions {
		if region == nil || !region.HasProtectedRange {
			continue
		}
		end := region.ProtectedEnd
		seen := map[*core.Node]bool{}
		for end != nil && !seen[end] && len(seen) < 32 && end != original {
			seen[end] = true
			if _, jump := end.Statement.(*statements.GOTOStatement); !jump || len(end.Next) != 1 {
				break
			}
			end = end.Next[0]
		}
		if end != original {
			continue
		}
		rows := [][2]int{{region.ProtectedStartPC, region.ProtectedEndPC}}
		for _, row := range region.SharedProtectedRanges {
			rows = append(rows, [2]int{int(row.StartPc), int(row.EndPc)})
		}
		typedBoundary, handlersKnown := false, true
		for _, next := range region.Next {
			if next != nil && next.IsCatchStart {
				if next.CatchHandler == nil || next.CatchHandler.CatchAll {
					// Catch-all structuring owns the normal cleanup copy as
					// input to the whole finally-domain proof. Its collector
					// boundary is not a typed handler's normal continuation.
					handlersKnown = false
					break
				}
				typedBoundary = true
				rows = append(rows, next.CatchHandler.ProtectedRanges...)
			}
		}
		if !typedBoundary || !handlersKnown {
			continue
		}
		ranges, valid := canonicalHandlerRanges(rows)
		if !valid {
			continue
		}
		proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
		if proof.block([]statements.Statement{replacement.Statement}, func(pc int) bool {
			return pc >= 0 && !finallyContains(ranges, pc)
		}, 0) {
			region.ProtectedEnd = replacement
		}
	}
}
