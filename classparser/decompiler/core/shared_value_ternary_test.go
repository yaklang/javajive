package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFactorSharedValueTernaryKeepsValueLeavesUnique(t *testing.T) {
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	first := values.NewJavaLiteral("first", types.NewJavaClass("java.lang.String"))
	second := values.NewJavaLiteral("second", types.NewJavaClass("java.lang.String"))
	b := values.NewTernaryExpression(values.NewJavaLiteral(0, boolType), first, second)
	a := values.NewTernaryExpression(values.NewJavaLiteral(1, boolType), first, b)
	rootOp, nextOp := &OpCode{}, &OpCode{}
	result, replacements, ok := factorSharedValueTernary(a, map[*OpCode]*values.TernaryExpression{rootOp: a, nextOp: b}, first, second)
	if !ok || result.TrueValue != first || result.FalseValue != second {
		t.Fatal("terminal values must appear once, outside the routing predicate")
	}
	if result.Condition != replacements[rootOp] || replacements[rootOp].FalseValue != replacements[nextOp] {
		t.Fatal("condition replacement callbacks lost their routing nodes")
	}
	if a.TrueValue != first || b.FalseValue != second {
		t.Fatal("factoring mutated original graph")
	}
}

func TestFactorSharedValueTernaryRejectsNonTreeRouting(t *testing.T) {
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	a, b := values.NewJavaLiteral(1, boolType), values.NewJavaLiteral(0, boolType)
	for _, kind := range []string{"shared condition", "cycle", "unknown leaf", "unreachable condition"} {
		t.Run(kind, func(t *testing.T) {
			child := values.NewTernaryExpression(a, a, b)
			root := values.NewTernaryExpression(b, child, b)
			built := map[*OpCode]*values.TernaryExpression{&OpCode{}: root, &OpCode{}: child}
			switch kind {
			case "shared condition":
				root.FalseValue = child
			case "cycle":
				child.TrueValue = root
			case "unknown leaf":
				child.TrueValue = values.NewJavaLiteral(2, boolType)
			case "unreachable condition":
				root.TrueValue = a
			}
			result, replacements, ok := factorSharedValueTernary(root, built, a, b)
			if ok || result != nil || replacements != nil {
				t.Fatal("unsafe routing graph accepted")
			}
		})
	}
}
