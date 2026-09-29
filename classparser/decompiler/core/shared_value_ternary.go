package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// factorSharedValueTernary separates control routing from value production.
// In `a ? shared() : (b ? shared() : other())`, every path first chooses one
// terminal block, then evaluates that block once. Reconstruct its predicate as
// `a ? true : (b ? true : false)` and emit one outer value ternary. Boolean
// reduction recovers a || b without changing test order or value effects.
//
// Conditions must form a tree (only terminal values may be shared). Refusing
// shared conditions bounds output linearly and avoids exponential expansion of
// a general decision DAG. All validation precedes publishing replacement nodes.
func factorSharedValueTernary(root *values.TernaryExpression, built map[*OpCode]*values.TernaryExpression, first, second values.JavaValue) (*values.TernaryExpression, map[*OpCode]*values.TernaryExpression, bool) {
	if root == nil || first == nil || second == nil || first == second {
		return nil, nil, false
	}
	conditions := map[*values.TernaryExpression]bool{}
	for _, condition := range built {
		conditions[condition] = true
	}
	replacements := map[*values.TernaryExpression]*values.TernaryExpression{}
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	var route func(values.JavaValue) (values.JavaValue, bool)
	route = func(value values.JavaValue) (values.JavaValue, bool) {
		if value == first {
			return values.NewJavaLiteral(1, boolType), true
		}
		if value == second {
			return values.NewJavaLiteral(0, boolType), true
		}
		condition, ok := value.(*values.TernaryExpression)
		if !ok || !conditions[condition] {
			return nil, false
		}
		if _, seen := replacements[condition]; seen {
			return nil, false
		}
		replacements[condition] = nil
		yes, ok := route(condition.TrueValue)
		if !ok {
			return nil, false
		}
		no, ok := route(condition.FalseValue)
		if !ok {
			return nil, false
		}
		next := values.NewTernaryExpression(condition.Condition, yes, no)
		replacements[condition] = next
		return next, true
	}
	predicate, ok := route(root)
	if !ok || len(replacements) != len(built) {
		return nil, nil, false
	}
	nextBuilt := make(map[*OpCode]*values.TernaryExpression, len(built))
	for op, condition := range built {
		nextBuilt[op] = replacements[condition]
	}
	return values.NewTernaryExpression(predicate, first, second), nextBuilt, true
}
