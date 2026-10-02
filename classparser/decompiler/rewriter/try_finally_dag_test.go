package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func finallySharedDecisionDAG(depth int) (values.JavaValue, []*values.FunctionCallExpression) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	var tail values.JavaValue = values.NewJavaLiteral(2, typ)
	calls := []*values.FunctionCallExpression{}
	for i := 0; i < depth; i++ {
		call := &values.FunctionCallExpression{ClassName: "example/Probe", FunctionName: "gate", Descriptor: "()Z", IsStatic: true, Kind: values.InvokeStatic, OriginPC: 100 + i, HasOriginPC: true, FuncType: types.NewJavaFuncType("()Z", nil, types.NewJavaPrimer(types.JavaBoolean))}
		calls = append(calls, call)
		// Two alternatives share one remaining graph. Proof memoization must not
		// turn this into execution memoization, copy effects or alter the graph.
		tail = values.NewTernaryExpression(call, tail, tail)
	}
	return tail, calls
}

func TestFinallyCoverageMemoizesEvidenceForSharedDAG(t *testing.T) {
	root, calls := finallySharedDecisionDAG(20)
	checks := map[int]int{}
	covered := func(pc int) bool { checks[pc]++; return pc >= 100 && pc < 120 }
	if !finallyCoveredValue(root, covered) {
		t.Fatal("twenty distinct protected effects must not exhaust proof through repeated paths")
	}
	for _, call := range calls {
		if checks[call.OriginPC] != 1 {
			t.Fatalf("PC%d checked%d times; proof expanded DAG", call.OriginPC, checks[call.OriginPC])
		}
	}
	if finallyCoveredValue(root, func(pc int) bool { return pc >= 101 && pc < 120 }) {
		t.Fatal("memo cannot escape its current domain or hide one unprotected effect")
	}
	if !finallyCoveredValue(root, covered) {
		t.Fatal("failed proof polluted original graph")
	}
	cursor := root
	for i := len(calls) - 1; i >= 0; i-- {
		x, ok := cursor.(*values.TernaryExpression)
		if !ok || x.Condition != calls[i] || x.TrueValue != x.FalseValue {
			t.Fatal("proof mutated or cloned original expression")
		}
		cursor = x.TrueValue
	}
}

func TestFinallyCoverageSharedDAGFailsClosed(t *testing.T) {
	for _, name := range []string{"cycle", "unprotected call", "missing origin", "typed nil", "unknown effect", "unique node budget"} {
		t.Run(name, func(t *testing.T) {
			root, calls := finallySharedDecisionDAG(20)
			switch name {
			case "cycle":
				root.(*values.TernaryExpression).TrueValue = root
			case "unprotected call":
				calls[10].OriginPC = 120
			case "missing origin":
				calls[10].HasOriginPC = false
			case "typed nil":
				root.(*values.TernaryExpression).Condition = (*values.FunctionCallExpression)(nil)
			case "unknown effect":
				root.(*values.TernaryExpression).Condition = &values.CustomValue{}
			case "unique node budget":
				root, _ = finallySharedDecisionDAG(300)
			}
			if finallyCoveredValue(root, func(pc int) bool { return pc >= 100 && pc < 500 }) && name != "unprotected call" {
				t.Fatal("unsupported/cyclic/incomplete graph must preserve the original tree")
			}
			if name == "unprotected call" && finallyCoveredValue(root, func(pc int) bool { return pc >= 100 && pc < 120 }) {
				t.Fatal("exclusive protected end admitted")
			}
		})
	}
}
