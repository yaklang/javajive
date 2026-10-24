package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A half-open exception interval may end at a continue GOTO, while another
// branch leaves the interval through an ordinary return expression. The
// interval's exclusive-end pointer is then not its normal continuation.
// Recover that continuation from the current CFG frontier only after binding
// the physical end GOTO to this method's recorded, sealed loop transfer.
func typedLoopTryNormalBoundary(manager *RewriteManager, region, start *core.Node) *core.Node {
	if manager == nil || region == nil || start == nil || !region.HasProtectedRange ||
		region.ProtectedStartPC < 0 || region.ProtectedStartPC >= region.ProtectedEndPC {
		return nil
	}
	end := region.ProtectedEnd
	if end == nil || !end.HasOriginPC || end.OriginPC != region.ProtectedEndPC || len(end.Next) != 1 {
		return nil
	}
	if _, jump := end.Statement.(*statements.GOTOStatement); !jump {
		return nil
	}
	header := end.Next[0]
	if header == nil || !header.HasOriginPC || header.OriginPC >= region.ProtectedStartPC {
		return nil
	}
	var rows [][2]int
	var handler *core.Node
	for _, next := range region.Next {
		if next == nil || !next.IsCatchStart {
			continue
		}
		if handler != nil || next.CatchHandler == nil || next.CatchHandler.CatchAll {
			return nil
		}
		handler = next
		rows = next.CatchHandler.ProtectedRanges
	}
	ranges, valid := canonicalHandlerRanges(rows)
	if !valid || len(ranges) != 1 || ranges[0] != [2]int{region.ProtectedStartPC, region.ProtectedEndPC} ||
		!typedHandlerTerminatesInCollectedRegion(handler, manager.DominatorMap) {
		return nil
	}
	transfers := map[*statements.CustomStatement]*core.Node{}
	for node, record := range manager.loopTransfers {
		if node == nil || record.kind != "continue" || record.target == nil ||
			!slices.Contains(manager.WhileNode, record.target) || len(record.target.Next) < 1 ||
			record.target.Next[0] != header || len(node.Next) != 1 || node.Next[0] != record.target {
			continue
		}
		leaf, ok := node.Statement.(*statements.CustomStatement)
		loop, loopKnown := record.target.Statement.(*statements.DoWhileStatement)
		kind, label, sealed := leaf.SourceTransferProtocol()
		if !ok || !loopKnown || loop == nil || !sealed || kind != record.kind || label != "" && label != loop.Label {
			continue
		}
		transfers[leaf] = record.target
	}
	if len(transfers) == 0 {
		return nil
	}
	inside := func(pc int) bool { return pc >= region.ProtectedStartPC && pc < region.ProtectedEndPC }
	outside := func(pc int) bool { return pc >= 0 && !inside(pc) }
	sawTransfer := false
	var encodedTargets map[*core.Node]bool
	proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}, transfer: func(leaf *statements.CustomStatement) bool {
		if transfers[leaf] == nil {
			return false
		}
		sawTransfer = true
		encodedTargets[transfers[leaf]] = true
		return true
	}}
	queue := []*core.Node{start}
	seen := map[*core.Node]bool{}
	var continuation *core.Node
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node == nil || len(seen) >= 512 || node.IsCatchStart {
			return nil
		}
		if seen[node] {
			continue
		}
		seen[node] = true
		if IsEndNode(node) {
			continue
		}
		if leaf, ok := node.Statement.(*statements.CustomStatement); ok && transfers[leaf] != nil {
			sawTransfer = true
			continue
		}
		if ret, ok := node.Statement.(*statements.ReturnStatement); ok && ret != nil &&
			node.HasOriginPC && ret.HasOriginPC && node.OriginPC == ret.OriginPC && outside(ret.OriginPC) {
			p := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if p.block([]statements.Statement{ret}, outside, 0) {
				if continuation != nil && continuation != node || len(node.EncodedJumps) != 0 {
					return nil
				}
				for _, next := range node.Next {
					if !IsEndNode(next) {
						return nil
					}
				}
				continuation = node
				continue
			}
		}
		encodedTargets = map[*core.Node]bool{}
		if condition, ok := node.Statement.(*statements.ConditionStatement); ok {
			if !node.HasOriginPC || !inside(node.OriginPC) || condition == nil || len(node.Next) != 2 ||
				!finallyCoveredValue(condition.Condition, inside) {
				return nil
			}
		} else if !proof.block([]statements.Statement{node.Statement}, inside, 0) {
			return nil
		}
		if isMethodExitTerminator(node) {
			for _, next := range node.Next {
				if !IsEndNode(next) {
					return nil
				}
			}
			continue
		}
		for _, next := range node.Next {
			if node.EncodedJumps[next] {
				if !encodedTargets[next] {
					return nil
				}
				continue
			}
			queue = append(queue, next)
		}
	}
	if !sawTransfer {
		return nil
	}
	return continuation
}
