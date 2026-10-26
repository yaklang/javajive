package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
	"slices"
)

// An inert literal stack-phi store can be shared by outer and inner routing
// edges. Give this edge its own store before dominance-based body collection.
// Both copies write the identical local at the original instruction boundary
// and reach the same continuation; no continuation effect is copied.
func splitSharedLiteralPhiStores(manager *RewriteManager, condition *core.Node) {
	changed := false
	for _, target := range slices.Clone(condition.Next) {
		assign, ok := target.Statement.(*statements.AssignStatement)
		if !ok || assign.IsFirst || assign.IsDeclare || !assign.HasOriginPC || !target.HasOriginPC || assign.OriginPC != target.OriginPC || len(target.Source) < 2 || len(target.Next) != 1 ||
			target.HideNext != nil || target.IsTryCatch || target.IsCatchStart || target.IsCircle || target.IsInCircle || target.LoopBreak || len(target.EncodedJumps) != 0 || encodedJumpTo(condition, target) || utils2.IsDominate(manager.DominatorMap, condition, target) {
			continue
		}
		ref, ok := assign.LeftValue.(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.IsParam || ref.Type() == nil {
			continue
		}
		primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
		if !ok || (primitive.Name != types.JavaBoolean && primitive.Name != types.JavaInteger) {
			continue
		}
		literal, ok := values.UnpackSoltValue(assign.JavaValue).(*values.JavaLiteral)
		if !ok || literal == nil || literal.Type() == nil {
			continue
		}
		literalType, ok := literal.Type().RawType().(*types.JavaPrimer)
		if !ok || literalType.Name != primitive.Name {
			continue
		}
		// Copy the same inert value on the selected original edge. Int
		// carriers retain the whole word; a later Z sink narrows bit zero.
		// No Boolean inference or 0/1 normalization is needed to split an
		// assignment of a constant to the same private local.
		switch primitive.Name {
		case types.JavaBoolean:
			if _, ok := literal.Data.(bool); !ok {
				continue
			}
		case types.JavaInteger:
			if _, ok := literal.Data.(int); !ok {
				continue
			}
		}
		exit := target.Next[0]
		if !sharedLiteralStoreKeepsOriginalDomain(manager, condition, target, exit) {
			// Legacy isolated callers without a complete exception table only
			// admit their existing inert terminal-return certificate.
			ret, ok := exit.Statement.(*statements.ReturnStatement)
			if !ok || ret == nil {
				continue
			}
			returned, returnRef := values.UnpackSoltValue(ret.JavaValue).(*values.JavaRef)
			if !ret.HasOriginPC || !exit.HasOriginPC || ret.OriginPC != exit.OriginPC || exit.HideNext != nil || exit.IsTryCatch || exit.IsCatchStart || exit.IsCircle || exit.IsInCircle || len(exit.EncodedJumps) != 0 || (!returnRef || !values.SameLocal(returned, ref)) || !sameProtectedMembership(manager.RootNode, condition, target) || !sameProtectedMembership(manager.RootNode, target, exit) {
				continue
			}
			terminal := true
			for _, next := range exit.Next {
				terminal = terminal && IsEndNode(next)
			}
			if !terminal {
				continue
			}
		}

		copy := *assign
		edge := manager.NewNode(&copy)
		edge.OriginPC, edge.HasOriginPC = target.OriginPC, target.HasOriginPC
		edge.AddNext(exit)
		replaceNextInPlace(condition, target, edge)
		changed = true
	}
	if changed {
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
	}
}

// Splitting a single local literal assignment is edge subdivision. The
// original continuation may perform arbitrary work: it is never copied or
// moved. A complete original handler table and exact PCs keep the new edge
// block in the same domain as the original selected store.
func sharedLiteralStoreKeepsOriginalDomain(manager *RewriteManager, condition, store, continuation *core.Node) bool {
	if manager == nil || !manager.originalHandlersKnown || condition == nil || store == nil || continuation == nil || !condition.HasOriginPC || !store.HasOriginPC || !continuation.HasOriginPC || condition.OriginPC < 0 || store.OriginPC < 0 || continuation.OriginPC < 0 || condition.OriginPC > 65535 || store.OriginPC > 65535 || continuation.OriginPC > 65535 || len(manager.originalHandlers) > 4096 {
		return false
	}
	if continuation.IsCatchStart || continuation.IsTryCatch || continuation.HideNext != nil {
		return false
	}
	for _, row := range manager.originalHandlers {
		contains := func(pc int) bool { return pc >= int(row.StartPc) && pc < int(row.EndPc) }
		if contains(condition.OriginPC) != contains(store.OriginPC) {
			return false
		}
	}
	return true
}
