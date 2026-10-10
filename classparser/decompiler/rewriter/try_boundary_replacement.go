package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A protected exclusive-end pointer is a semantic reference, even when its
// original GOTO was detached from the normal CFG. Replacing a condition must
// keep that reference on the current structured continuation. Do not infer a
// boundary from node IDs or the first uncovered instruction: a folded input
// may still throw inside the protected region. The complete replacement must
// stay outside every original interval of the affected handlers.
func retargetProtectedIfBoundary(regions []*core.Node, original, replacement *core.Node) {
	if original == nil || replacement == nil {
		return
	}
	if _, ok := replacement.Statement.(*statements.IfStatement); !ok {
		return
	}
	retargetProtectedSourceBoundary(regions, original, replacement)
}

func retargetProtectedSourceBoundary(regions []*core.Node, original, replacement *core.Node) {
	for _, region := range regions {
		if region == nil || !region.HasProtectedRange {
			continue
		}
		end := region.ProtectedEnd
		seen := map[*core.Node]bool{}
		for end != nil && !seen[end] && len(seen) < 32 && end != original {
			seen[end] = true
			if _, jump := end.Statement.(*statements.GOTOStatement); !jump || len(end.Next) != 1 {
				break
			}
			end = end.Next[0]
		}
		if end != original {
			continue
		}
		rows := [][2]int{{region.ProtectedStartPC, region.ProtectedEndPC}}
		for _, row := range region.SharedProtectedRanges {
			rows = append(rows, [2]int{int(row.StartPc), int(row.EndPc)})
		}
		typedBoundary, handlersKnown := false, true
		for _, next := range region.Next {
			if next != nil && next.IsCatchStart {
				if next.CatchHandler == nil || next.CatchHandler.CatchAll {
					// Catch-all structuring owns the normal cleanup copy as
					// input to the whole finally-domain proof. Its collector
					// boundary is not a typed handler's normal continuation.
					handlersKnown = false
					break
				}
				typedBoundary = true
				rows = append(rows, next.CatchHandler.ProtectedRanges...)
			}
		}
		if !typedBoundary || !handlersKnown {
			continue
		}
		ranges, valid := canonicalHandlerRanges(rows)
		if !valid {
			continue
		}
		proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
		if proof.block([]statements.Statement{replacement.Statement}, func(pc int) bool {
			return pc >= 0 && !finallyContains(ranges, pc)
		}, 0) {
			region.ProtectedEnd = replacement
		}
	}
}

// RetargetCollapsedConditionBoundary runs at the value-ternary collapse, before
// the original node loses its normal edges. The one distinct current successor and the
// original consumer witness identify the replacement; text, PCs or graph IDs
// alone cannot do so. The common proof also checks every folded effect against
// every affected typed handler interval and refuses catch-all cleanup domains.
func RetargetCollapsedConditionBoundary(regions []*core.Node, original, replacement *core.Node) {
	if len(regions) == 0 || original == nil || replacement == nil || len(original.Next) < 1 || len(original.Next) > 2 {
		return
	}
	for _, next := range original.Next {
		if next != replacement {
			return
		}
	}
	condition, ok := original.Statement.(*statements.ConditionStatement)
	if !ok || condition == nil || condition.Callback == nil || condition.Condition == nil {
		return
	}
	var operand values.JavaValue
	switch statement := replacement.Statement.(type) {
	case *statements.AssignStatement:
		if statement == nil {
			return
		}
		operand = statement.JavaValue
	case *statements.ExpressionStatement:
		if statement == nil {
			return
		}
		operand = statement.Expression
	default:
		return
	}
	if replacement.SourceConditionNode != original {
		// A reconstructed ternary tree deliberately has no single legacy root
		// condition ID. Its sealed callback still binds each condition identity
		// into the immediate consumer expression, including duplicate branch edges
		// that converge on the same node. Prove one exact condition occurrence.
		if replacement.SourceConditionNode != nil || !condition.TernaryChainArm || !collapsedTernaryConditionOnce(operand, condition.Condition) {
			return
		}
	}
	retargetProtectedSourceBoundary(regions, original, replacement)
}

func collapsedTernaryConditionOnce(root, condition values.JavaValue) bool {
	remaining, matches := 512, 0
	active := map[values.JavaValue]bool{}
	var walk func(values.JavaValue) bool
	walk = func(value values.JavaValue) bool {
		remaining--
		if remaining < 0 || value == nil || active[value] {
			return false
		}
		active[value] = true
		defer delete(active, value)
		if ternary, ok := value.(*values.TernaryExpression); ok && ternary != nil && ternary.Condition == condition {
			matches++
			if matches > 1 {
				return false
			}
		}
		children, known := values.Children(value)
		if !known {
			return false
		}
		for _, child := range children {
			if child == nil {
				continue
			}
			if !walk(child) {
				return false
			}
		}
		return true
	}
	return walk(root) && matches == 1
}
