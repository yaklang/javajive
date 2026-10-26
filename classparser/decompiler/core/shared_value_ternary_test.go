package core

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
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
	result, replacements, ok := factorSharedValueTernary(a, map[*OpCode]*values.TernaryExpression{rootOp: a, nextOp: b}, first, second, nil)
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
			result, replacements, ok := factorSharedValueTernary(root, built, a, b, nil)
			if ok || result != nil || replacements != nil {
				t.Fatal("unsafe routing graph accepted")
			}
		})
	}
}

// Compare the original decision graph's interpreter with the planned source
// tree for every short truth table and every injected condition failure. Long
// ladders additionally enforce one occurrence of each original condition.
func TestFactorSharedDecisionRegionsPreservesTraceAndLinearSize(t *testing.T) {
	for _, shape := range []string{"selected gate", "positive exit", "ladder"} {
		t.Run(shape, func(t *testing.T) {
			boolType := types.NewJavaPrimer(types.JavaBoolean)
			first := values.NewJavaLiteral("first", types.NewJavaClass("java.lang.Object"))
			second := values.NewJavaLiteral("second", types.NewJavaClass("java.lang.Object"))
			built := map[*OpCode]*values.TernaryExpression{}
			gates := map[values.JavaValue]int{}
			node := func(yes, no values.JavaValue) *values.TernaryExpression {
				gate := values.NewJavaLiteral(0, boolType)
				gates[gate] = len(gates)
				n := values.NewTernaryExpression(gate, yes, no)
				built[&OpCode{Id: len(built)}] = n
				return n
			}
			var root *values.TernaryExpression
			if shape == "ladder" {
				var tail values.JavaValue = first
				for i := 0; i < 20; i++ {
					b := node(tail, second)
					tail = node(tail, b)
				}
				root = tail.(*values.TernaryExpression)
			} else {
				shared := node(first, second)
				if shape == "positive exit" {
					root = node(node(first, shared), node(shared, first))
				} else {
					root = node(node(shared, second), node(shared, second))
				}
			}
			result, replaced, ok := factorSharedValueTernary(root, built, first, second, nil)
			if !ok || len(replaced) != len(built) {
				t.Fatal("closed decision regions rejected")
			}
			occurrences := map[values.JavaValue]int{}
			nodes := 0
			var visit func(values.JavaValue)
			visit = func(v values.JavaValue) {
				nodes++
				if _, gate := gates[v]; gate {
					occurrences[v]++
					return
				}
				if n, yes := v.(*values.TernaryExpression); yes {
					visit(n.Condition)
					visit(n.TrueValue)
					visit(n.FalseValue)
				}
			}
			visit(result)
			if nodes > len(gates)*10+4 {
				t.Fatal("nonlinear source expansion", nodes, len(gates))
			}
			for gate := range gates {
				if occurrences[gate] != 1 {
					t.Fatal("duplicated or missing condition", gates[gate], occurrences[gate])
				}
			}
			evaluate := func(start values.JavaValue, mask uint64, failAt int) (values.JavaValue, []int, bool) {
				trace := []int{}
				failed := false
				var eval func(values.JavaValue) any
				eval = func(v values.JavaValue) any {
					if v == first || v == second {
						return v
					}
					if id, gate := gates[v]; gate {
						trace = append(trace, id)
						if id == failAt {
							failed = true
						}
						return mask&(uint64(1)<<id) != 0
					}
					if n, ok := v.(*values.TernaryExpression); ok {
						truth := eval(n.Condition)
						if failed {
							return nil
						}
						if truth.(bool) {
							return eval(n.TrueValue)
						}
						return eval(n.FalseValue)
					}
					return v.(*values.JavaLiteral).Data.(int) != 0
				}
				v := eval(start)
				if failed {
					return nil, trace, true
				}
				return v.(values.JavaValue), trace, false
			}
			masks := []uint64{}
			if len(gates) <= 8 {
				for i := uint64(0); i < 1<<len(gates); i++ {
					masks = append(masks, i)
				}
			} else {
				for _, mask := range []uint64{0, ^uint64(0), 0x5555555555555555, 0x3333333333333333} {
					masks = append(masks, mask)
				}
				for i := 0; i < len(gates); i++ {
					masks = append(masks, uint64(1)<<i, ^(uint64(1) << i))
				}
			}
			for _, mask := range masks {
				for failure := -1; failure < len(gates); failure++ {
					want, wt, wf := evaluate(root, mask, failure)
					got, gt, gf := evaluate(result, mask, failure)
					if want != got || wf != gf || !reflect.DeepEqual(wt, gt) {
						t.Fatalf("mask=%x failure=%d: traces %v/%v failures %v/%v", mask, failure, wt, gt, wf, gf)
					}
				}
			}
		})
	}
}

func TestFactorSharedDecisionRegionsRejectsCrossEntryAndBudget(t *testing.T) {
	for _, kind := range []string{"cross entry", "scan budget", "during partition", "allocation budget", "cancelled", "duplicate ownership", "too many conditions"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaBoolean)
			first, second := values.NewJavaLiteral(1, typ), values.NewJavaLiteral(0, typ)
			built := map[*OpCode]*values.TernaryExpression{}
			node := func(a, b values.JavaValue) *values.TernaryExpression {
				n := values.NewTernaryExpression(values.NewJavaLiteral(1, typ), a, b)
				built[&OpCode{Id: len(built)}] = n
				return n
			}
			shared := node(first, second)
			left, right := node(shared, second), node(shared, second)
			root := node(left, right)
			var budget *workbudget.Budget
			switch kind {
			case "cross entry":
				other := node(second, first)
				left.FalseValue = other
				right.TrueValue = other
				right.FalseValue = shared
			case "scan budget":
				budget = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "during partition":
				budget = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 20})
			case "allocation budget":
				budget = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				budget = workbudget.New(ctx, workbudget.Limits{})
			case "duplicate ownership":
				built[&OpCode{Id: 99}] = shared
			case "too many conditions":
				for i := 0; i < 513; i++ {
					root = node(root, second)
				}
			}
			before := map[*values.TernaryExpression][3]values.JavaValue{}
			for _, n := range built {
				before[n] = [3]values.JavaValue{n.Condition, n.TrueValue, n.FalseValue}
			}
			result, replaced, ok := factorSharedValueTernary(root, built, first, second, budget)
			if ok || result != nil || replaced != nil {
				t.Fatal("unproved decision graph accepted")
			}
			for n, want := range before {
				if got := ([3]values.JavaValue{n.Condition, n.TrueValue, n.FalseValue}); got != want {
					t.Fatal("failed proof mutated original")
				}
			}
		})
	}
}
