package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// Dominance alone also includes the code after a loop. Require a bounded path
// back to the same recorded natural-loop wrapper before treating a node as
// part of its body. Choose the nearest owner independently for both nodes so
// an inner-loop exit cannot borrow an outer loop's continue permission.
func sharedSwitchContinuationLoop(manager *RewriteManager, owner, target *core.Node) *core.Node {
	if manager == nil || !manager.LoopRegionReducible || len(manager.WhileNode) > 64 {
		return nil
	}
	inside := func(start, loop *core.Node) (bool, bool) {
		if start == nil || loop == nil || len(loop.Next) != 1 || start == loop || !utils.IsDominate(manager.DominatorMap, loop, start) {
			return false, true
		}
		seen := map[*core.Node]bool{}
		pending := []*core.Node{start}
		for len(pending) != 0 {
			if len(pending) > 1024 || len(seen) >= 256 {
				return false, false
			}
			n := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if n == loop {
				return true, true
			}
			if n == nil || seen[n] || !utils.IsDominate(manager.DominatorMap, loop, n) {
				continue
			}
			seen[n] = true
			if len(n.Next) > 1024 {
				return false, false
			}
			pending = append(pending, n.Next...)
		}
		return false, true
	}
	nearest := func(node *core.Node) *core.Node {
		var result *core.Node
		for _, loop := range manager.WhileNode {
			member, complete := inside(node, loop)
			// An unfinished inner-loop search must not grant permission to
			// treat its continue as belonging to an outer loop.
			if !complete {
				return nil
			}
			if !member {
				continue
			}
			if result == nil || utils.IsDominate(manager.DominatorMap, result, loop) {
				result = loop
			} else if !utils.IsDominate(manager.DominatorMap, loop, result) {
				return nil
			}
		}
		return result
	}
	loop := nearest(owner)
	if loop == nil || loop != nearest(target) || target == loop.Next[0] {
		return nil
	}
	return loop
}
