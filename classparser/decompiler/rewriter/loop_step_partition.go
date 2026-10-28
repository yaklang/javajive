package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// A shared for-loop update is a continuation for each incoming edge, not the
// fall-through of whichever inner decision is collected first. Give every
// original incoming edge its own nonthrowing local update before collecting
// decisions. The edge still reaches the same loop header and executes exactly
// one update; normal body effects and bypassing continues stay distinct.
func partitionSharedLoopSteps(m *RewriteManager) {
	if m == nil || !m.LoopRegionReducible || !m.originalHandlersKnown || len(m.WhileNode) > 64 || len(m.originalHandlers) > 512 {
		return
	}
	for _, loop := range m.WhileNode {
		if loop == nil || len(loop.Source) > 256 {
			continue
		}
		for _, tail := range slices.Clone(loop.Source) {
			copyStep, sources := proveSharedLoopStep(m, loop, tail)
			if copyStep == nil {
				continue
			}
			for _, from := range sources {
				private := m.NewNode(copyStep())
				private.OriginPC, private.HasOriginPC, private.IsInCircle = tail.OriginPC, true, tail.IsInCircle
				replaceNextInPlace(from, tail, private)
				private.AddNext(loop)
			}
			tail.RemoveAllNext()
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
		}
	}
}

func proveSharedLoopStep(m *RewriteManager, loop, tail *core.Node) (func() statements.Statement, []*core.Node) {
	if m == nil || loop == nil || len(loop.Next) != 1 || tail == loop.Next[0] {
		return nil, nil
	}
	if _, wrapper := loop.Statement.(*statements.DoWhileStatement); !wrapper {
		return nil, nil
	}
	copyStep, pure := switchLoopLocalStep(tail)
	if !pure || tail == nil || !tail.HasOriginPC || tail.OriginPC < 0 || tail.OriginPC > 65535 || tail.HideNext != nil || tail.IsCatchStart || tail.IsTryCatch || tail.HasPendingTryDispatch() || tail.HasProtectedRange || len(tail.EncodedJumps) != 0 || len(tail.Next) != 1 || tail.Next[0] != loop || len(tail.Source) < 2 || len(tail.Source) > 128 || !utils.IsDominate(m.DominatorMap, loop, tail) {
		return nil, nil
	}
	for _, row := range m.originalHandlers {
		if row.StartPc >= row.EndPc {
			return nil, nil
		}
	}
	sources := map[*core.Node]bool{}
	for _, from := range tail.Source {
		if from == nil || sources[from] || !from.HasOriginPC || from.OriginPC < 0 || from.OriginPC > 65535 || from.HideNext != nil || from.IsCatchStart || from.HasPendingTryDispatch() || len(from.EncodedJumps) != 0 || !slices.Contains(from.Next, tail) || !utils.IsDominate(m.DominatorMap, loop, from) {
			return nil, nil
		}
		for _, row := range m.originalHandlers {
			if (from.OriginPC >= int(row.StartPc) && from.OriginPC < int(row.EndPc)) != (tail.OriginPC >= int(row.StartPc) && tail.OriginPC < int(row.EndPc)) {
				return nil, nil
			}
		}
		sources[from] = true
	}
	// The reverse links are not a sufficient inventory. Check every live edge
	// (including hidden/encoded transfers) before retiring the shared node.
	live := protectedCommitLiveGraph(m.RootNode)
	if live == nil {
		return nil, nil
	}
	seen := map[*core.Node]bool{}
	for _, node := range live {
		if node.HideNext == tail || node.EncodedJumps[tail] {
			return nil, nil
		}
		for _, next := range node.Next {
			if next == tail {
				if !sources[node] || seen[node] {
					return nil, nil
				}
				seen[node] = true
			}
		}
	}
	if len(seen) != len(sources) {
		return nil, nil
	}
	return copyStep, slices.Clone(tail.Source)
}
