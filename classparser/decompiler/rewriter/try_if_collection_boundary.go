package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
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
			if end := typedEarlyReturnNormalFrontier(region, start, region.Next, manager.DominatorMap); end != nil {
				stops[end] = true
			} else if end := typedTryNormalBoundary(region, start, region.Next, manager.DominatorMap); end != nil {
				stops[end] = true
			} else if end := typedLoopTryNormalBoundary(manager, region, start); end != nil {
				stops[end] = true
			}
		}
	}
	return stops
}

// An exception interval can end at an early RETURN, rather than the common
// normal continuation. A return of an already evaluated local/constant can
// stay in its original if arm: the transfer cannot throw into this handler.
// Detaching it leaves an empty arm and falsely turns an early return into a
// fall-through to a different continuation. Calls, loads, casts, allocations,
// division and unknown renderers must retain their original effect domains.
func protectedArmCanOwnReturn(node *core.Node) bool {
	if node == nil || !node.HasOriginPC || len(node.EncodedJumps) != 0 {
		return false
	}
	ret, known := node.Statement.(*statements.ReturnStatement)
	if !known || ret == nil || !ret.HasOriginPC || ret.OriginPC != node.OriginPC {
		return false
	}
	for _, next := range node.Next {
		if !IsEndNode(next) {
			return false
		}
	}
	effects, _ := values.InspectValue(ret.JavaValue)
	if effects&values.BarrierEffects != 0 {
		return false
	}
	proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
	if !proof.block([]statements.Statement{ret}, func(int) bool { return false }, 0) {
		return false
	}
	return true
}
