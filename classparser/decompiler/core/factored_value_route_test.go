package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestFactoredArrayArmRetainsOriginalTerminalOwnership(t *testing.T) {
	for _, variant := range []string{"complete", "mutable targets changed", "copied selector", "changed condition", "swapped arms", "duplicated call", "wrong terminal", "missing graph", "unproved entry", "missing original stack witness", "foreign invocation", "interior alternate entry", "interior exceptional entry", "decision after allocation", "different handler", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			g := t09Graph(7, t09Edge{0, 1, EdgeFallthrough}, t09Edge{1, 2, EdgeFallthrough}, t09Edge{1, 4, EdgeTaken}, t09Edge{2, 3, EdgeFallthrough}, t09Edge{2, 4, EdgeTaken}, t09Edge{3, 6, EdgeFallthrough}, t09Edge{4, 5, EdgeFallthrough}, t09Edge{5, 6, EdgeFallthrough})
			for i, op := range g.Nodes {
				op.Instr, op.CurrentOffset = &Instruction{OpCode: OP_NOP}, uint16(i*10)
			}
			entry, inner, other, allocation, invoke := g.Nodes[1], g.Nodes[2], g.Nodes[3], g.Nodes[4], g.Nodes[5]
			entry.Instr.OpCode, inner.Instr.OpCode = OP_IFEQ, OP_IFEQ
			entry.TernaryChainArm, inner.TernaryChainArm = true, true
			allocation.Instr.OpCode, invoke.Instr.OpCode = OP_ANEWARRAY, OP_INVOKESTATIC
			call := &values.FunctionCallExpression{ClassName: "sample.Factory", FunctionName: "consume", Descriptor: "([Ljava/lang/Object;)Ljava/lang/Object;", Kind: values.InvokeStatic, IsStatic: true, OriginPC: 50, HasOriginPC: true}
			alternative := values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))
			truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			inside := values.NewTernaryExpression(truth, alternative, call)
			root := values.NewTernaryExpression(truth, inside, call)
			selector, _, ok := factorSharedValueTernary(root, map[*OpCode]*values.TernaryExpression{entry: root, inner: inside}, alternative, call, nil)
			if !ok || selector.ConditionFromOp != 0 {
				t.Fatal("expected a synthetic factored selector")
			}
			d := &Decompiler{semanticCFG: g, opCodes: g.Nodes, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			d.opcodeToSimulateStack = map[*OpCode]*StackSimulationImpl{}
			for _, op := range g.Nodes {
				d.opcodeToSimulateStack[op] = nil
			}
			d.recordFactoredValueRoute(selector, entry, other, invoke)
			switch variant {
			case "mutable targets changed":
				entry.Target, allocation.Target = []*OpCode{other}, []*OpCode{other}
			case "copied selector":
				copy := *selector
				selector = &copy
			case "changed condition":
				selector.Condition = values.NewJavaLiteral(false, truth.Type())
			case "swapped arms":
				selector.TrueValue, selector.FalseValue = selector.FalseValue, selector.TrueValue
			case "duplicated call":
				selector.TrueValue = call
			case "wrong terminal":
				delete(d.factoredValueRoutes, selector)
				d.recordFactoredValueRoute(selector, entry, invoke, other)
			case "missing graph":
				d.semanticCFG = nil
			case "unproved entry":
				entry.TernaryChainArm = false
			case "missing original stack witness":
				d.opcodeToSimulateStack = nil
			case "foreign invocation":
				copy := *call
				copy.Descriptor = "()Ljava/lang/Object;"
				d.invokeFuncCall[invoke] = &copy
			case "interior alternate entry", "interior exceptional entry":
				kind := EdgeFallthrough
				if variant == "interior exceptional entry" {
					kind = EdgeException
				}
				g.Edges = append(g.Edges, SemanticEdge{From: other, To: invoke, Kind: kind})
			case "decision after allocation":
				g.Edges = append(g.Edges, SemanticEdge{From: allocation, To: other, Kind: EdgeTaken})
			case "different handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 40, EndPc: 60, HandlerPc: 60}}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			arm, origin := d.branchArraySelectedSourceArm(selector, call, allocation)
			want := variant == "complete" || variant == "mutable targets changed"
			if (arm == call && origin == entry) != want {
				t.Fatalf("accepted=%v, want=%v", arm != nil, want)
			}
			if selector.ConditionFromOp != 0 {
				t.Fatal("synthetic predicate acquired physical branch identity")
			}
		})
	}
}
