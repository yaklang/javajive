package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
	"slices"
)

// A typed catch that resumes at the normal update gives that update a second
// entry. The ordinary try collector already stops there by dominance. Retain
// the same boundary while collecting nested decisions, so a protected skip
// can leave its value/commit region without executing an unassigned commit.
func typedSharedResumeBoundary(m *RewriteManager, region, start *core.Node) *core.Node {
	if m == nil || !m.originalHandlersKnown || len(m.originalHandlers) > 512 || region == nil || start == nil || !region.HasProtectedRange || len(region.Next) > 17 {
		return nil
	}
	var ranges [][2]int
	handlers := map[int]bool{}
	for _, n := range region.Next {
		if n == nil || !n.IsCatchStart {
			continue
		}
		if n.CatchHandler == nil || n.CatchHandler.CatchAll || !n.HasOriginPC || n.OriginPC != n.CatchHandler.EntryPC || handlers[n.OriginPC] {
			return nil
		}
		rows, ok := canonicalHandlerRanges(n.CatchHandler.ProtectedRanges)
		if !ok || len(rows) > 32 {
			return nil
		}
		if ranges == nil {
			ranges = rows
		} else if !slices.Equal(ranges, rows) {
			return nil
		}
		handlers[n.OriginPC] = true
	}
	if len(handlers) == 0 || !slices.Contains(ranges, [2]int{region.ProtectedStartPC, region.ProtectedEndPC}) {
		return nil
	}
	// Every participating target has exactly these physical intervals. Any
	// competing dispatch or omitted interval makes the packet incomplete.
	physical := map[int][][2]int{}
	types := map[int]uint16{}
	for _, row := range m.originalHandlers {
		if row.StartPc >= row.EndPc {
			return nil
		}
		overlaps := false
		for _, r := range ranges {
			overlaps = overlaps || int(row.StartPc) < r[1] && int(row.EndPc) > r[0]
		}
		target := int(row.HandlerPc)
		if !handlers[target] {
			if overlaps {
				return nil
			}
			continue
		}
		if row.CatchType == 0 || types[target] != 0 && types[target] != row.CatchType {
			return nil
		}
		types[target] = row.CatchType
		physical[target] = append(physical[target], [2]int{int(row.StartPc), int(row.EndPc)})
	}
	for target := range handlers {
		rows, ok := canonicalHandlerRanges(physical[target])
		if !ok || !slices.Equal(rows, ranges) {
			return nil
		}
	}
	end := region.ProtectedEnd
	seen := map[*core.Node]bool{}
	for end != nil {
		if seen[end] || len(seen) >= 32 {
			return nil
		}
		seen[end] = true
		if _, jump := end.Statement.(*statements.GOTOStatement); !jump {
			break
		}
		if end.HideNext != nil || len(end.EncodedJumps) != 0 || len(end.Next) != 1 {
			return nil
		}
		end = end.Next[0]
	}
	if end == nil || !end.HasOriginPC || end.IsCatchStart || end.HideNext != nil || len(end.EncodedJumps) != 0 || utils2.IsDominate(m.DominatorMap, start, end) {
		return nil
	}
	outside := func(pc int) bool { return pc >= 0 && !finallyContains(ranges, pc) }
	p := outsideHandlerDomainProof(ranges)
	if !outside(end.OriginPC) || !p.block([]statements.Statement{end.Statement}, outside, 0) || !typedSplitNormalBoundaryClosed(start, end, ranges) {
		return nil
	}
	return end
}
