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
		copyStep, ok := switchLoopLocalStep(latch)
		if !ok || len(latch.Next) != 1 || latch.Next[0] != loop {
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
				increment := manager.NewNode(copyStep())
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

// JVM IINC uses both ++/-- and local = local +/- constant in the expression
// graph. Their proof is the same: a bound int local, a literal int displacement,
// no throwing operands and no declaration/field/array write. Preserve Java int
// overflow and the exact displacement instead of guessing a unit step.
func switchLoopLocalStep(node *core.Node) (func() statements.Statement, bool) {
	if node == nil {
		return nil, false
	}
	var expr *values.JavaExpression
	var assigned *values.JavaRef
	switch step := node.Statement.(type) {
	case *values.JavaExpression:
		if step == nil || step.Op != values.INC && step.Op != values.DEC {
			return nil, false
		}
		expr = step
	case *statements.AssignStatement:
		if step == nil || step.IsDeclare || step.IsFirst || step.Final || step.ArrayMember != nil {
			return nil, false
		}
		assigned, _ = step.LeftValue.(*values.JavaRef)
		expr, _ = step.JavaValue.(*values.JavaExpression)
		if assigned == nil || expr == nil || expr.Op != values.ADD && expr.Op != values.SUB {
			return nil, false
		}
	default:
		return nil, false
	}
	if len(expr.Values) != 2 {
		return nil, false
	}
	local, ok := expr.Values[0].(*values.JavaRef)
	if !ok || local == nil || local.Id == nil || local.IsThis || local.CustomValue != nil || local.StackVar != nil || local.Type() == nil || assigned != nil && assigned != local {
		return nil, false
	}
	primitive, ok := local.Type().RawType().(*types.JavaPrimer)
	if !ok || primitive.Name != types.JavaInteger {
		return nil, false
	}
	amount, ok := expr.Values[1].(*values.JavaLiteral)
	if !ok || amount == nil || amount.Type() == nil {
		return nil, false
	}
	word, ok := amount.Data.(int)
	if !ok || int64(word) < -2147483648 || int64(word) > 2147483647 {
		return nil, false
	}
	amountType, ok := amount.Type().RawType().(*types.JavaPrimer)
	if !ok || amountType.Name != types.JavaInteger || assigned == nil && word != 1 {
		return nil, false
	}
	return func() statements.Statement {
		copied := values.NewBinaryExpression(local, amount, expr.Op, expr.Typ)
		if assigned != nil {
			return statements.NewAssignStatement(local, copied, false)
		}
		return copied
	}, true
}
