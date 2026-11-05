package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// An early normal join can precede a later loop step also reached by continue
// arms. Pick the unique upstream join in the switch-owned acyclic region.
// Statement allocation order is not execution order; paths through the loop
// header or another owner cannot establish this ordering.
func firstSwitchJoin(manager *RewriteManager, owner *core.Node, candidates []*core.Node) *core.Node {
	if manager == nil || owner == nil || len(candidates) < 2 || len(candidates) > 64 {
		return nil
	}
	remaining := 65536
	var result *core.Node
	for _, start := range candidates {
		state := map[*core.Node]uint8{}
		var visit func(*core.Node) bool
		visit = func(n *core.Node) bool {
			remaining--
			if n == nil || remaining < 0 || len(state) >= 512 || len(n.Next) > 512 {
				return false
			}
			if n == owner || !utils.IsDominate(manager.DominatorMap, owner, n) {
				return true
			}
			if state[n] != 0 {
				return state[n] == 2
			}
			if n.IsCatchStart || n.HasPendingTryDispatch() || n.HideNext != nil || len(n.EncodedJumps) != 0 {
				return false
			}
			state[n] = 1
			for _, next := range n.Next {
				if !visit(next) {
					return false
				}
			}
			state[n] = 2
			return true
		}
		if !visit(start) {
			return nil
		}
		all := true
		for _, candidate := range candidates {
			all = all && state[candidate] == 2
		}
		if all {
			if result != nil {
				return nil
			}
			result = start
		}
	}
	return result
}
