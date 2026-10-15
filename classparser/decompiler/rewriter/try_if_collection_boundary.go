package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
)

// Dominance describes normal reachability, not exception-table ownership.
// Before collecting an if arm, preserve each enclosing typed try's proved
// normal continuation. Otherwise the arm can consume an unprotected return
// expression and make its exceptions reach a handler that never covered it.
// Match actual boundary pointers and complete effect trees, not the first PC
// beyond an interval: a folded RETURN can still contain a protected producer.
func typedProtectedIfCollectionStops(manager *RewriteManager, condition *core.Node) map[*core.Node]bool {
	stops := map[*core.Node]bool{}
	if manager == nil || condition == nil || !condition.HasOriginPC {
		return stops
	}
	st, ok := condition.Statement.(*statements.ConditionStatement)
	if !ok || st == nil || st.Condition == nil {
		return stops
	}
	for _, region := range manager.TryNodes {
		if region == nil || !region.HasProtectedRange || condition.OriginPC < region.ProtectedStartPC ||
			condition.OriginPC >= region.ProtectedEndPC || !utils2.IsDominate(manager.DominatorMap, region, condition) {
			continue
		}
		inside := func(pc int) bool { return pc >= region.ProtectedStartPC && pc < region.ProtectedEndPC }
		if !finallyCoveredValue(st.Condition, inside) {
			continue
		}
		for _, start := range region.Next {
			if start == nil || start.IsCatchStart {
				continue
			}
			if end := typedTryNormalBoundary(region, start, region.Next, manager.DominatorMap); end != nil {
				stops[end] = true
			} else if end := typedLoopTryNormalBoundary(manager, region, start); end != nil {
				stops[end] = true
			}
		}
	}
	return stops
}
