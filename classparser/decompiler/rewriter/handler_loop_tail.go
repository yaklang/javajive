package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// A catch can bypass a try's normal continuation through a shared loop step.
// Dominance-based body collection stops at that shared step; treating both
// boundaries as ordinary try fall-through loses the handler's continue.
// Subdivide the handler edge with a private copy of the nonthrowing local step
// and its already proved continue, before collecting the catch. The original
// normal path retains its step and all its preceding effects.
func materializeHandlerLoopTails(manager *RewriteManager, region *core.Node) {
	if manager == nil || region == nil || !region.HasProtectedRange || region.ProtectedStartPC < 0 || region.ProtectedStartPC >= region.ProtectedEndPC || manager.DominatorMap == nil {
		return
	}
	type plan struct {
		from, tail, loop *core.Node
		copyStep         func() statements.Statement
		label            string
	}
	var plans []plan
	remaining := 4096
	for _, handler := range region.Next {
		if handler == nil || !handler.IsCatchStart || handler.CatchHandler == nil || !handler.HasOriginPC || handler.OriginPC != handler.CatchHandler.EntryPC {
			continue
		}
		ranges, valid := canonicalHandlerRanges(handler.CatchHandler.ProtectedRanges)
		if !valid || !slices.Contains(ranges, [2]int{region.ProtectedStartPC, region.ProtectedEndPC}) {
			continue
		}
		seen := map[*core.Node]bool{}
		queue := []*core.Node{handler}
		for len(queue) > 0 {
			n := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			remaining--
			if n == nil || remaining < 0 || len(seen) > 512 {
				return
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			for _, next := range n.Next {
				remaining--
				if remaining < 0 {
					return
				}
				if next == nil || next.IsCatchStart || n.HideNext != nil || encodedJumpTo(n, next) {
					continue
				}
				if slices.Contains(manager.DominatorMap[n], next) {
					queue = append(queue, next)
					continue
				}
				step, loop, label := handlerSharedLoopStep(manager, handler, next)
				if step != nil {
					plans = append(plans, plan{n, next, loop, step, label})
				}
			}
		}
	}
	for _, p := range plans {
		step := manager.NewNode(p.copyStep())
		step.OriginPC, step.HasOriginPC = p.tail.OriginPC, p.tail.HasOriginPC
		transfer := manager.NewNode(statements.NewSourceTransferStatement("continue", p.label))
		transfer.IsJmp = true
		manager.recordLoopTransfer(transfer, p.loop, "continue", p.from)
		replaceNextInPlace(p.from, p.tail, step)
		step.AddNext(transfer)
		transfer.AddNext(p.loop)
	}
	if len(plans) > 0 {
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
	}
}

func handlerSharedLoopStep(manager *RewriteManager, handler, tail *core.Node) (func() statements.Statement, *core.Node, string) {
	if tail == nil || !tail.HasOriginPC || tail.HideNext != nil || tail.IsCatchStart || tail.IsTryCatch || tail.HasProtectedRange || tail.IsJmp || len(tail.EncodedJumps) != 0 || len(tail.Next) != 1 {
		return nil, nil, ""
	}
	copyStep, pure := switchLoopLocalStep(tail)
	if !pure {
		return nil, nil, ""
	}
	transfer := tail.Next[0]
	if transfer == nil || !transfer.IsJmp || transfer.HideNext != nil || len(transfer.Next) != 1 {
		return nil, nil, ""
	}
	record, known := manager.loopTransfers[transfer]
	loop := record.target
	leaf, typed := transfer.Statement.(*statements.CustomStatement)
	if !known || record.kind != "continue" || loop == nil || transfer.Next[0] != loop || !slices.Contains(manager.WhileNode, loop) || !typed || leaf == nil {
		return nil, nil, ""
	}
	owner, ok := loop.Statement.(*statements.DoWhileStatement)
	kind, label, sealed := leaf.SourceTransferProtocol()
	if !ok || owner == nil || !sealed || kind != "continue" || label != "" && label != owner.Label ||
		!utils.IsDominate(manager.DominatorMap, loop, handler) || !utils.IsDominate(manager.DominatorMap, loop, tail) {
		return nil, nil, ""
	}
	// Moving the existing unqualified transfer into an inner loop would
	// change its target. Retain the conservative proof unless it is labelled.
	if label == "" {
		for _, inner := range manager.WhileNode {
			if inner != loop && utils.IsDominate(manager.DominatorMap, loop, inner) && utils.IsDominate(manager.DominatorMap, inner, handler) {
				return nil, nil, ""
			}
		}
	}
	return copyStep, loop, label
}
