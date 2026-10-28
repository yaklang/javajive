package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// A bottom test shared by nested-loop exits is not an ordinary switch merge.
// Subdivide its incoming edges before dominance-based collectors consume them.
// Each selected edge still performs the same test exactly once. A shared local
// update immediately before it likewise retains one evaluation per original
// incoming edge, including abrupt completion and the original handler domain.
// Copy-stable operands exclude allocation, lambdas and opaque source emitters.
func partitionSharedLoopLatches(manager *RewriteManager) {
	if manager == nil || !manager.LoopRegionReducible || len(manager.WhileNode) > 64 {
		return
	}
	for _, loop := range manager.WhileNode {
		plan := proveSharedLoopLatch(manager, loop)
		if plan == nil {
			continue
		}
		for _, edge := range plan.edges {
			from := edge.from
			if edge.copyStep != nil {
				step := manager.NewNode(edge.copyStep())
				step.OriginPC, step.HasOriginPC, step.IsInCircle = edge.prefix.OriginPC, edge.prefix.HasOriginPC, edge.prefix.IsInCircle
				replaceNextInPlace(from, edge.prefix, step)
				step.AddNext(plan.tail)
				from = step
			}
			condition := *plan.condition
			condition.Condition = copyPureLatchCondition(plan.condition.Condition)
			private := manager.NewNode(&condition)
			private.OriginPC, private.HasOriginPC, private.IsInCircle = plan.tail.OriginPC, plan.tail.HasOriginPC, plan.tail.IsInCircle
			private.AddNext(plan.no)
			private.AddNext(plan.yes)
			private.TrueNode = func() *core.Node { return private.Next[1] }
			private.FalseNode = func() *core.Node { return private.Next[0] }
			replaceNextInPlace(from, plan.tail, private)
			manager.IfNodes = append(manager.IfNodes, private)
		}
		for prefix := range plan.prefixes {
			prefix.RemoveAllNext()
		}
		plan.tail.RemoveAllNext()
		manager.IfNodes = slices.DeleteFunc(manager.IfNodes, func(n *core.Node) bool { return n == plan.tail })
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
	}
}

type latchEdge struct {
	from, prefix *core.Node
	copyStep     func() statements.Statement
}

type sharedLoopLatch struct {
	tail, yes, no *core.Node
	condition     *statements.ConditionStatement
	edges         []latchEdge
	prefixes      map[*core.Node]bool
}

func proveSharedLoopLatch(manager *RewriteManager, loop *core.Node) *sharedLoopLatch {
	if manager == nil || !manager.originalHandlersKnown || len(manager.originalHandlers) > 512 || loop == nil || len(loop.Next) != 1 || len(loop.Source) > 256 {
		return nil
	}
	for _, row := range manager.originalHandlers {
		if row.StartPc >= row.EndPc {
			return nil
		}
	}
	var backedges []*core.Node
	for _, source := range loop.Source {
		if source == nil || !slices.Contains(source.Next, loop) {
			return nil
		}
		if utils.IsDominate(manager.DominatorMap, loop, source) {
			backedges = append(backedges, source)
		}
	}
	if len(backedges) != 1 {
		return nil
	}
	tail := backedges[0]
	condition, ok := tail.Statement.(*statements.ConditionStatement)
	if !ok || condition == nil || tail == loop.Next[0] || !tail.HasOriginPC || tail.HideNext != nil || tail.IsCatchStart || tail.IsTryCatch || tail.HasProtectedRange || len(tail.EncodedJumps) != 0 || len(tail.Next) != 2 || len(tail.Source) < 2 || len(tail.Source) > 128 || copyPureLatchCondition(condition.Condition) == nil {
		return nil
	}
	yes, no := ifBranchNodes(tail)
	if yes == nil || no == nil || yes == no || yes != loop && no != loop {
		return nil
	}
	plan := &sharedLoopLatch{tail: tail, yes: yes, no: no, condition: condition, prefixes: map[*core.Node]bool{}}
	sameDomain := func(a, b *core.Node) bool {
		if a.OriginPC < 0 || a.OriginPC > 65535 || b.OriginPC < 0 || b.OriginPC > 65535 {
			return false
		}
		for _, row := range manager.originalHandlers {
			left := a.OriginPC >= int(row.StartPc) && a.OriginPC < int(row.EndPc)
			right := b.OriginPC >= int(row.StartPc) && b.OriginPC < int(row.EndPc)
			if left != right {
				return false
			}
		}
		return true
	}
	validEdge := func(from, to *core.Node) bool {
		return from != nil && from != to && from.HasOriginPC && from.HideNext == nil && !from.IsCatchStart && !from.IsTryCatch && len(from.EncodedJumps) == 0 && slices.Contains(from.Next, to) && utils.IsDominate(manager.DominatorMap, loop, from) && sameDomain(from, to)
	}
	for _, from := range tail.Source {
		if !validEdge(from, tail) || len(plan.edges) >= 128 {
			return nil
		}
		if len(from.Next) == 1 && len(from.Source) > 1 {
			copyStep := switchLoopAssignmentStep(from)
			if copyStep == nil || len(from.Source) > 128 {
				return nil
			}
			for _, predecessor := range from.Source {
				if !validEdge(predecessor, from) || len(plan.edges) >= 128 {
					return nil
				}
				plan.edges = append(plan.edges, latchEdge{from: predecessor, prefix: from, copyStep: copyStep})
			}
			plan.prefixes[from] = true
		} else {
			plan.edges = append(plan.edges, latchEdge{from: from})
		}
	}
	return plan
}

// This phase only copies local/literal comparisons and Boolean composition.
// Calls, unboxing, fields, allocations, updates and incomplete/cyclic operands
// cannot authorize tail subdivision. No expression is evaluated by the proof.
func copyPureLatchCondition(value values.JavaValue) values.JavaValue {
	remaining := 128
	var copyValue func(values.JavaValue, int) values.JavaValue
	copyValue = func(value values.JavaValue, depth int) values.JavaValue {
		remaining--
		if value == nil || remaining < 0 || depth > 16 {
			return nil
		}
		if value == values.JavaNull {
			return value
		}
		switch v := value.(type) {
		case *values.SlotValue:
			if v != nil {
				return copyValue(v.GetValue(), depth+1)
			}
		case *values.JavaRef:
			if v != nil && v.Id != nil && v.CustomValue == nil && v.StackVar == nil {
				return v
			}
		case *values.JavaLiteral:
			if v != nil {
				return v
			}
		case *values.JavaExpression:
			if v == nil || len(v.Values) < 1 || len(v.Values) > 2 {
				return nil
			}
			switch v.Op {
			case values.Not:
				if len(v.Values) != 1 {
					return nil
				}
			case values.EQ, values.NEQ, values.LT, values.GT, values.LTE, values.GTE, values.LOGICAL_AND, values.LOGICAL_OR:
				if len(v.Values) != 2 {
					return nil
				}
			default:
				return nil
			}
			copy := *v
			copy.Values = nil
			for _, child := range v.Values {
				operand := copyValue(child, depth+1)
				if operand == nil {
					return nil
				}
				copy.Values = append(copy.Values, operand)
			}
			return &copy
		}
		return nil
	}
	return copyValue(value, 0)
}
