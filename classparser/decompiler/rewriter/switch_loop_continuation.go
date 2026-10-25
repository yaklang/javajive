package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// A switch arm can leave through the enclosing for-loop step instead of the
// switch's normal continuation. Removing the bytecode GOTO does not turn that
// edge into fall-through: the normal continuation may still contain effects.
// Materialize this boundary before case/if collection makes its edges opaque.
// Only the nonthrowing local step is copied; the arm's effects stay in place.
func materializeSwitchLatchContinues(manager *RewriteManager, loop *core.Node) {
	changed := false
	for _, latch := range slices.Clone(loop.Source) {
		step, ok := asLatchIncExpr(latch)
		if !ok || len(step.Values) != 2 || len(latch.Next) != 1 || latch.Next[0] != loop {
			continue
		}
		local, ok := step.Values[0].(*values.JavaRef)
		if !ok || local == nil || local.Id == nil || local.IsThis || local.CustomValue != nil || local.StackVar != nil || local.Type() == nil {
			continue
		}
		primitive, ok := local.Type().RawType().(*types.JavaPrimer)
		if !ok || primitive.Name != types.JavaInteger {
			continue
		}
		amount, literal := step.Values[1].(*values.JavaLiteral)
		if !literal || amount == nil || amount.Data != 1 {
			continue
		}
		if !utils.IsDominate(manager.DominatorMap, loop, latch) {
			continue
		}
		for _, owner := range manager.SwitchNode {
			merge := owner.MergeNode
			if !owner.SwitchPrepared || merge == nil || merge == latch ||
				!utils.IsDominate(manager.DominatorMap, loop, owner) ||
				!utils.IsDominate(manager.DominatorMap, owner, merge) ||
				!switchContinuationReachesLatch(merge, latch) {
				continue
			}
			for _, predecessor := range slices.Clone(latch.Source) {
				if predecessor == owner || !utils.IsDominate(manager.DominatorMap, owner, predecessor) ||
					utils.IsDominate(manager.DominatorMap, merge, predecessor) {
					continue
				}
				increment := manager.NewNode(values.NewBinaryExpression(step.Values[0], step.Values[1], step.Op, step.Typ))
				transfer := manager.NewNode(statements.NewSourceTransferStatement("continue", ""))
				transfer.IsJmp = true
				manager.recordLoopTransfer(transfer, loop, "continue", predecessor)
				replaceNextInPlace(predecessor, latch, increment)
				increment.AddNext(transfer)
				transfer.AddNext(loop)
				changed = true
			}
		}
	}
	if changed {
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
	}
}

// Certify a finite continuation whose every CFG path reaches the step. A
// competing exit, cycle, hidden transfer or exhausted bound is not a proof.
func switchContinuationReachesLatch(start, latch *core.Node) bool {
	state := map[*core.Node]uint8{}
	var visit func(*core.Node) bool
	visit = func(node *core.Node) bool {
		if node == latch {
			return true
		}
		if node == nil || state[node] == 1 {
			return false
		}
		if state[node] == 2 {
			return true
		}
		if len(state) >= 256 || len(node.Next) == 0 || len(node.Next) > 2 ||
			node.HideNext != nil || len(node.EncodedJumps) != 0 || node.IsJmp ||
			node.IsCircle || node.IsCatchStart || node.IsTryCatch {
			return false
		}
		// Opaque containers can encode abrupt completion inside their bodies.
		// This proof runs on the still-unstructured normal continuation.
		switch node.Statement.(type) {
		case *statements.ConditionStatement, *statements.AssignStatement, *statements.ExpressionStatement, *values.JavaExpression:
		default:
			return false
		}
		state[node] = 1
		for _, next := range node.Next {
			if !visit(next) {
				return false
			}
		}
		state[node] = 2
		return true
	}
	return start != nil && latch != nil && start != latch && visit(start)
}
