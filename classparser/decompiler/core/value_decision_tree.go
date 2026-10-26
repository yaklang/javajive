package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A tree of original conditions can select arbitrarily many terminal values.
// Sharing a terminal does not duplicate its evaluation: exactly one leaf is
// reached. Sharing a condition is different; unfolding that DAG could expand
// exponentially, and must instead use a separately proved region factorization.
// This certificate keeps every original condition once and bounds the number
// of emitted leaf occurrences by conditions+1, without changing any operand.
func closedValueDecisionTree(root *values.TernaryExpression, built map[*OpCode]*values.TernaryExpression, leaves map[*OpCode]values.JavaValue, budget *workbudget.Budget) bool {
	if root == nil || len(built) == 0 || len(built) > 512 || len(leaves) == 0 || len(leaves) > len(built)+1 {
		return false
	}
	if budget != nil && (budget.Charge(workbudget.CounterGraphScans, int64(len(built)+len(leaves))) != nil || budget.CheckAlloc(int64(len(built)+len(leaves))*128) != nil) {
		return false
	}
	conditions := map[*values.TernaryExpression]bool{}
	for op, value := range built {
		if op == nil || value == nil || conditions[value] {
			return false
		}
		conditions[value] = true
	}
	terminals := map[values.JavaValue]bool{}
	for op, value := range leaves {
		if op == nil || value == nil {
			return false
		}
		if node, ok := value.(*values.TernaryExpression); ok && conditions[node] {
			return false
		}
		terminals[value] = true
	}
	seen := map[*values.TernaryExpression]bool{}
	used := map[values.JavaValue]bool{}
	pending := []values.JavaValue{root}
	for len(pending) > 0 {
		if budget != nil && budget.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		value := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if terminals[value] {
			used[value] = true
			continue
		}
		node, ok := value.(*values.TernaryExpression)
		if !ok || !conditions[node] || seen[node] || node.Condition == nil {
			return false
		}
		seen[node] = true
		pending = append(pending, node.FalseValue, node.TrueValue)
	}
	return len(seen) == len(conditions) && len(used) == len(terminals)
}
