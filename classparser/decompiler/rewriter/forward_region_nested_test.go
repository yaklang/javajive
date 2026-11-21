package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNestedMultiExitForwardRegionChainedExits(t *testing.T) {
	// Outer decision `owner` contains two inner decisions:
	// - true arm (`if0`) goes to either `work1` or bypasses to `work2` (E1)
	// - false arm (`if1`) goes to either `work1` or bypasses past `work2` to `exit` (E2)
	// with `work1 -> work2 -> exit -> return`.
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
	owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
	if0 := core.NewNode(&statements.ConditionStatement{Condition: truth})
	if1 := core.NewNode(&statements.ConditionStatement{Condition: truth})
	a0 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
	b0 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
	work1 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(10, types.NewJavaPrimer(types.JavaInteger))))
	work2 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(20, types.NewJavaPrimer(types.JavaInteger))))
	exit := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(30, types.NewJavaPrimer(types.JavaInteger))))
	ret := core.NewNode(&statements.ReturnStatement{})

	all := []*core.Node{root, owner, if0, if1, a0, b0, work1, work2, exit, ret}
	for i, n := range all {
		n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
	}
	root.AddNext(owner)
	owner.AddNext(if0)
	owner.AddNext(if1)
	if0.AddNext(a0)
	if0.AddNext(work2)
	a0.AddNext(work1)
	if1.AddNext(b0)
	if1.AddNext(exit)
	b0.AddNext(work1)
	work1.AddNext(work2)
	work2.AddNext(exit)
	exit.AddNext(ret)
	for _, n := range []*core.Node{owner, if0, if1} {
		n := n
		n.TrueNode = func() *core.Node { return n.Next[0] }
		n.FalseNode = func() *core.Node { return n.Next[1] }
	}

	m := NewRootStatementManager(root)
	m.SetOriginalExceptionTable(nil)
	m.currentNodeId = 100
	m.IfNodes = []*core.Node{owner, if0, if1}
	m.DominatorMap = GenerateDominatorTree(root)

	// Bottom-up rewrite: if1 registers E2 (outer), if0 registers E1 (inner), then owner wraps both.
	for _, cond := range []*core.Node{if1, if0, owner} {
		materializeForwardRegionExits(m, cond)
		if err := IfRewriter(m, cond); err != nil {
			t.Fatalf("IfRewriter(%d) failed: %v", cond.Id, err)
		}
		m.DominatorMap = GenerateDominatorTree(root)
	}
	if len(m.forwardRegions) != 0 {
		t.Fatalf("expected all pending forward regions to be consumed, left=%d", len(m.forwardRegions))
	}
	outerWrapper := root.Next[0]
	outerLoop, ok := outerWrapper.Statement.(*statements.DoWhileStatement)
	if !ok || len(outerLoop.Body) < 2 {
		t.Fatalf("expected outer DoWhileStatement wrapper, got %T", outerWrapper.Statement)
	}
	innerLoop, ok := outerLoop.Body[0].(*statements.DoWhileStatement)
	if !ok {
		t.Fatalf("expected inner DoWhileStatement nested inside outer region, got %T", outerLoop.Body[0])
	}
	if outerLoop.Label == "" || innerLoop.Label == "" || outerLoop.Label == innerLoop.Label {
		t.Fatalf("expected distinct labels on nested forward regions, got inner=%q outer=%q", innerLoop.Label, outerLoop.Label)
	}
	if len(outerWrapper.Next) != 1 || outerWrapper.Next[0] != exit {
		t.Fatalf("expected outer wrapper to fall through solely to exit, got %v", outerWrapper.Next)
	}
}

func TestNestedMultiExitForwardRegionParallelSiblingJoin(t *testing.T) {
	// Parallel sibling branches inside `owner`:
	// - owner -> inner1 -> [left, right]
	// - owner -> inner2 -> [left, right]
	// Neither left nor right is dominated by inner1/inner2 and neither reaches the other;
	// post-join discovery registers `join` on owner, seals left->join as `break L_join`,
	// and wraps inner1/inner2..right inside L_join.
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
	owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
	inner1 := core.NewNode(&statements.ConditionStatement{Condition: truth})
	inner2 := core.NewNode(&statements.ConditionStatement{Condition: truth})
	left := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
	right := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))))
	join := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(4, types.NewJavaPrimer(types.JavaInteger))))
	ret := core.NewNode(&statements.ReturnStatement{})

	all := []*core.Node{root, owner, inner1, inner2, left, right, join, ret}
	for i, n := range all {
		n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
	}
	root.AddNext(owner)
	owner.AddNext(inner1)
	owner.AddNext(inner2)
	inner1.AddNext(left)
	inner1.AddNext(right)
	inner2.AddNext(left)
	inner2.AddNext(right)
	left.AddNext(join)
	right.AddNext(join)
	join.AddNext(ret)
	for _, n := range []*core.Node{owner, inner1, inner2} {
		n := n
		n.TrueNode = func() *core.Node { return n.Next[0] }
		n.FalseNode = func() *core.Node { return n.Next[1] }
	}

	m := NewRootStatementManager(root)
	m.SetOriginalExceptionTable(nil)
	m.currentNodeId = 100
	m.IfNodes = []*core.Node{owner, inner1, inner2}
	m.DominatorMap = GenerateDominatorTree(root)

	for _, cond := range []*core.Node{inner2, inner1, owner} {
		if err := IfRewriter(m, cond); err != nil {
			t.Fatalf("IfRewriter(%d) failed on sibling join: %v", cond.Id, err)
		}
		m.DominatorMap = GenerateDominatorTree(root)
	}
	if len(m.forwardRegions) != 0 {
		t.Fatalf("expected all forward regions consumed, left=%d", len(m.forwardRegions))
	}
	wrapper := root.Next[0]
	if _, ok := wrapper.Statement.(*statements.DoWhileStatement); !ok {
		t.Fatalf("expected outer DoWhileStatement for sibling join, got %T", wrapper.Statement)
	}
	if len(wrapper.Next) != 1 || wrapper.Next[0] != join {
		t.Fatalf("expected single normal continuation at join, got %v", wrapper.Next)
	}
}
