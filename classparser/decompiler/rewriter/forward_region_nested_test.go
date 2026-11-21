package rewriter

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
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

func TestForwardRegionParallelSiblingTailsAndLoopNesting(t *testing.T) {
	t.Run("loop_nested_parallel_exits_no_false_cycle", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		loopHdr := core.NewNode(statements.NewDoWhileStatement(truth, nil))
		loopHdr.IsCircle = true
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		inner1 := core.NewNode(&statements.ConditionStatement{Condition: truth})
		inner2 := core.NewNode(&statements.ConditionStatement{Condition: truth})
		a0 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		b0 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
		work1 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(10, types.NewJavaPrimer(types.JavaInteger))))
		work2 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(20, types.NewJavaPrimer(types.JavaInteger))))
		exit := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(30, types.NewJavaPrimer(types.JavaInteger))))
		joinCont := core.NewNode(statements.NewSourceTransferStatement("continue", ""))
		loopExit := core.NewNode(statements.NewReturnStatement(nil))

		all := []*core.Node{root, loopHdr, owner, inner1, inner2, a0, b0, work1, work2, exit, joinCont, loopExit}
		for i, n := range all {
			n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
		}
		root.AddNext(loopHdr)
		loopHdr.AddNext(owner)
		loopHdr.AddNext(loopExit)
		owner.AddNext(inner1)
		owner.AddNext(inner2)
		inner1.AddNext(a0)
		inner1.AddNext(work2)
		a0.AddNext(work1)
		inner2.AddNext(b0)
		inner2.AddNext(exit)
		b0.AddNext(work1)
		work1.AddNext(work2)
		work2.AddNext(exit)
		exit.AddNext(joinCont)
		joinCont.AddNext(loopHdr)
		joinCont.EncodedJumps = map[*core.Node]bool{loopHdr: true}

		m := NewRootStatementManager(root)
		m.SetOriginalExceptionTable(nil)
		m.currentNodeId = 100
		m.WhileNode = []*core.Node{loopHdr}
		m.IfNodes = []*core.Node{owner, inner1, inner2}
		m.DominatorMap = GenerateDominatorTree(root)

		for _, cond := range []*core.Node{inner2, inner1, owner} {
			if err := IfRewriter(m, cond); err != nil {
				t.Fatalf("IfRewriter(%d) failed with loop back-edge: %v", cond.Id, err)
			}
			m.DominatorMap = GenerateDominatorTree(root)
		}
		if len(m.forwardRegions) != 0 {
			t.Fatalf("expected all forward regions consumed, left=%d", len(m.forwardRegions))
		}
		if _, ok := loopHdr.Next[0].Statement.(*statements.DoWhileStatement); !ok {
			t.Fatalf("expected wrapped DoWhileStatement inside loop, got %T", loopHdr.Next[0].Statement)
		}
	})

	t.Run("nondominated_outer_exit_wraps_dominated_inner_first", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		guard := core.NewNode(&statements.ConditionStatement{Condition: truth})
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		trueArm := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		falseArm := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
		innerJoin := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))))
		outerCond := core.NewNode(&statements.ConditionStatement{Condition: truth})
		bypass := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(4, types.NewJavaPrimer(types.JavaInteger))))
		ret1 := core.NewNode(statements.NewReturnStatement(nil))
		ret2 := core.NewNode(statements.NewReturnStatement(nil))

		all := []*core.Node{root, guard, owner, trueArm, falseArm, innerJoin, outerCond, bypass, ret1, ret2}
		for i, n := range all {
			n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
		}
		root.AddNext(guard)
		guard.AddNext(owner)
		guard.AddNext(bypass)
		bypass.AddNext(outerCond)
		owner.AddNext(trueArm)
		owner.AddNext(falseArm)
		trueArm.AddNext(innerJoin)
		falseArm.AddNext(innerJoin)
		innerJoin.AddNext(outerCond)
		outerCond.AddNext(ret1)
		outerCond.AddNext(ret2)

		m := NewRootStatementManager(root)
		m.SetOriginalExceptionTable(nil)
		m.currentNodeId = 100
		m.IfNodes = []*core.Node{guard, owner, outerCond}
		m.DominatorMap = GenerateDominatorTree(root)

		ownerWrapper := core.NewNode(statements.NewIfStatement(truth, nil, nil))
		ownerWrapper.Id, ownerWrapper.OriginPC, ownerWrapper.HasOriginPC = 50, 50, true
		m.replaceStructuredNode(owner, ownerWrapper)
		ownerWrapper.RemoveAllNext()
		ownerWrapper.AddNext(innerJoin)
		m.DominatorMap = GenerateDominatorTree(root)

		// Register dominated innerJoin first and non-dominated outerCond second:
		// finishForwardRegion must close innerJoin first (not fail walking out into raw outerCond).
		m.forwardRegions = map[*core.Node][]*forwardRegion{
			owner: {
				{exit: outerCond, label: "LOOP_OUTER"},
				{exit: innerJoin, label: "LOOP_INNER"},
			},
		}
		if err := finishForwardRegion(m, owner, ownerWrapper); err != nil {
			t.Fatalf("finishForwardRegion failed with non-dominated outer exit: %v", err)
		}
		wrapped := guard.Next[0]
		outerDo, ok := wrapped.Statement.(*statements.DoWhileStatement)
		if !ok || outerDo.Label != "LOOP_OUTER" || len(wrapped.Next) != 1 || wrapped.Next[0] != outerCond {
			t.Fatalf("expected outer wrapper ending at outerCond (%p), got %T next=%v", outerCond, wrapped.Statement, wrapped.Next)
		}
	})

	t.Run("if_rewriter_stops_at_pending_forward_exit", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		outer := core.NewNode(&statements.ConditionStatement{Condition: truth})
		inner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		arm1 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		arm2 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
		pendingExit := core.NewNode(&statements.ConditionStatement{Condition: truth})
		ret := core.NewNode(statements.NewReturnStatement(nil))

		all := []*core.Node{root, outer, inner, arm1, arm2, pendingExit, ret}
		for i, n := range all {
			n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
		}
		root.AddNext(outer)
		outer.AddNext(inner)
		outer.AddNext(ret)
		inner.AddNext(arm1)
		inner.AddNext(arm2)
		arm1.AddNext(pendingExit)
		arm2.AddNext(pendingExit)
		pendingExit.AddNext(ret)
		pendingExit.AddNext(ret)

		m := NewRootStatementManager(root)
		m.SetOriginalExceptionTable(nil)
		m.currentNodeId = 100
		m.forwardRegions = map[*core.Node][]*forwardRegion{
			outer: {{exit: pendingExit, label: "LOOP_OUTER"}},
		}
		m.DominatorMap = GenerateDominatorTree(root)

		if err := IfRewriter(m, inner); err != nil {
			t.Fatalf("IfRewriter failed: %v", err)
		}
		ifNode := outer.Next[0]
		if _, ok := ifNode.Statement.(*statements.IfStatement); !ok {
			t.Fatalf("expected IfStatement, got %T", ifNode.Statement)
		}
		if len(ifNode.Next) != 1 || ifNode.Next[0] != pendingExit {
			t.Fatalf("expected IfRewriter to stop at pendingExit without swallowing it, next=%v", ifNode.Next)
		}
	})
}

func TestForwardRegionAbruptIfAndNegativeCycles(t *testing.T) {
	t.Run("structured_all_abrupt_if_exit_succeeds", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		ret1 := &statements.ReturnStatement{OriginPC: 30, HasOriginPC: true}
		ret2 := &statements.ReturnStatement{OriginPC: 40, HasOriginPC: true}
		abruptIf := core.NewNode(statements.NewIfStatement(truth, []statements.Statement{ret1}, []statements.Statement{ret2}))
		abruptIf.OriginPC, abruptIf.HasOriginPC = 20, true
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		exit := core.NewNode(statements.NewReturnStatement(nil))
		exit.OriginPC, exit.HasOriginPC = 50, true
		root.AddNext(abruptIf)

		m := NewRootStatementManager(root)
		m.forwardRegions = map[*core.Node][]*forwardRegion{
			owner: {{exit: exit, label: "LOOP_0"}},
		}
		if err := finishForwardRegion(m, owner, abruptIf); err != nil {
			t.Fatalf("expected structured all-abrupt IfStatement exit to succeed, got %v", err)
		}
	})

	t.Run("non_abrupt_or_break_if_len_next_zero_fails_closed", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		ret := &statements.ReturnStatement{OriginPC: 30, HasOriginPC: true}
		brk := statements.NewSourceTransferStatement("break", "LOOP_0")
		badIf := core.NewNode(statements.NewIfStatement(truth, []statements.Statement{ret}, []statements.Statement{brk}))
		badIf.OriginPC, badIf.HasOriginPC = 20, true
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		exit := core.NewNode(statements.NewReturnStatement(nil))
		root.AddNext(badIf)

		m := NewRootStatementManager(root)
		m.forwardRegions = map[*core.Node][]*forwardRegion{
			owner: {{exit: exit, label: "LOOP_0"}},
		}
		err := finishForwardRegion(m, owner, badIf)
		if err == nil || !strings.Contains(err.Error(), "normal continuations") {
			t.Fatalf("expected non-abrupt IfStatement with len(Next)==0 to fail closed, got %v", err)
		}
	})

	t.Run("genuine_forward_exit_cycle_fails_closed", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		start := core.NewNode(statements.NewIfStatement(truth, nil, nil))
		e1 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		e2 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
		root.AddNext(start)
		start.AddNext(e1)
		start.AddNext(e2)
		start.EncodedJumps = map[*core.Node]bool{e2: true}
		e1.AddNext(e2)
		e2.AddNext(e1)

		m := NewRootStatementManager(root)
		m.forwardRegions = map[*core.Node][]*forwardRegion{
			owner: {
				{exit: e1, label: "LOOP_0"},
				{exit: e2, label: "LOOP_1"},
			},
		}
		err := finishForwardRegion(m, owner, start)
		if err == nil || !strings.Contains(err.Error(), "exits form a cycle") {
			t.Fatalf("expected genuine forward exit cycle to fail closed, got %v", err)
		}
	})

	t.Run("two_nondominated_unrelated_exits_rejected", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		g1 := core.NewNode(&statements.ConditionStatement{Condition: truth})
		g2 := core.NewNode(&statements.ConditionStatement{Condition: truth})
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		e1 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		e2 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
		ret1 := core.NewNode(&statements.ReturnStatement{OriginPC: 60, HasOriginPC: true})
		ret2 := core.NewNode(&statements.ReturnStatement{OriginPC: 70, HasOriginPC: true})

		all := []*core.Node{root, g1, g2, owner, e1, e2, ret1, ret2}
		for i, n := range all {
			n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
		}
		ret1.Statement = &statements.ReturnStatement{OriginPC: ret1.OriginPC, HasOriginPC: true}
		ret2.Statement = &statements.ReturnStatement{OriginPC: ret2.OriginPC, HasOriginPC: true}
		root.AddNext(g1)
		g1.AddNext(g2)
		g1.AddNext(e1)
		g2.AddNext(owner)
		g2.AddNext(e2)
		owner.AddNext(e1)
		owner.AddNext(e2)
		e1.AddNext(ret1)
		e2.AddNext(ret2)

		m := NewRootStatementManager(root)
		m.SetOriginalExceptionTable(nil)
		m.DominatorMap = GenerateDominatorTree(root)
		m.forwardRegions = map[*core.Node][]*forwardRegion{
			owner: {{exit: e1, label: "LOOP_0"}},
		}
		if closedForwardRegion(m, owner, e2) {
			t.Fatalf("expected closedForwardRegion to reject non-dominated exit alongside unordered sibling")
		}
	})
}

func TestRewriteVarDoWhileForwardRegionEscapingSlot(t *testing.T) {
	intT := types.NewJavaPrimer(types.JavaInteger)
	boolFalse := values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean))
	boolTrue := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))

	t.Run("nested_do_while_false_unifies_escaping_slot", func(t *testing.T) {
		sharedID := utils.NewRootVariableId()
		sharedID.SetName("var5")
		makeRef := func() *values.JavaRef {
			r := values.NewJavaRef(sharedID, nil, intT)
			r.VarUid = "uid_shared"
			return r
		}

		storeA := statements.NewAssignStatement(makeRef(), values.NewJavaLiteral(1, intT), true)
		brk := statements.NewSourceTransferStatement("break", "LOOP_1")
		inner := statements.NewDoWhileStatement(boolFalse, []statements.Statement{storeA, brk})
		inner.Label = "LOOP_2"

		storeB := statements.NewAssignStatement(makeRef(), values.NewJavaLiteral(2, intT), true)
		outer := statements.NewDoWhileStatement(boolFalse, []statements.Statement{inner, storeB})
		outer.Label = "LOOP_1"

		ret := statements.NewReturnStatement(makeRef())
		sts := []statements.Statement{outer, ret}

		RewriteVar(&sts, 1, nil)

		if len(sts) < 3 {
			t.Fatalf("expected hoisted declaration before LOOP_1 + return, got len=%d", len(sts))
		}
		idA := storeA.LeftValue.(*values.JavaRef).Id
		idB := storeB.LeftValue.(*values.JavaRef).Id
		idRet := ret.JavaValue.(*values.JavaRef).Id
		if idA == nil || idA != idB || idB != idRet {
			t.Fatalf("expected unified VariableId across LOOP_2, LOOP_1, and return: idA=%p idB=%p idRet=%p", idA, idB, idRet)
		}
		if storeA.IsFirst || storeB.IsFirst || !declaresIdentity(sts[0], idA) {
			t.Fatalf("expected hoisted declaration at root without re-declaring stores")
		}
	})

	t.Run("non_escaping_inner_local_not_prebound", func(t *testing.T) {
		id1 := utils.NewRootVariableId()
		id1.SetName("var5")
		id2 := utils.NewRootVariableId()
		id2.SetName("var5")
		ref1 := values.NewJavaRef(id1, nil, intT)
		ref1.VarUid = "uid_dead"
		ref2 := values.NewJavaRef(id2, nil, intT)
		ref2.VarUid = "uid_dead"

		storeA := statements.NewAssignStatement(ref1, values.NewJavaLiteral(1, intT), true)
		inner := statements.NewDoWhileStatement(boolFalse, []statements.Statement{storeA})
		storeB := statements.NewAssignStatement(ref2, values.NewJavaLiteral(2, intT), true)
		outer := statements.NewDoWhileStatement(boolFalse, []statements.Statement{inner, storeB})
		ret := statements.NewReturnStatement(values.NewJavaLiteral(0, intT))
		sts := []statements.Statement{outer, ret}

		RewriteVar(&sts, 1, nil)
		if len(sts) != 2 || sts[0] != outer {
			t.Fatalf("did not expect non-escaping local to hoist declaration above LOOP_1, got len=%d", len(sts))
		}
	})

	t.Run("do_while_true_loop_not_prebound", func(t *testing.T) {
		id1 := utils.NewRootVariableId()
		id1.SetName("var5")
		makeRef := func() *values.JavaRef {
			r := values.NewJavaRef(id1, nil, intT)
			r.VarUid = "uid_loop"
			return r
		}
		store := statements.NewAssignStatement(makeRef(), values.NewJavaLiteral(7, intT), true)
		loop := statements.NewDoWhileStatement(boolTrue, []statements.Statement{store})
		ret := statements.NewReturnStatement(makeRef())
		sts := []statements.Statement{loop, ret}

		scope := NewScope(1, &sts)
		idMap := map[*utils.VariableId]*utils.VariableId{}
		prebindEscapingDoWhileSlots(scope, loop, []statements.Statement{ret}, idMap, "C", "m")
		if len(scope.assignedMap) != 0 || len(idMap) != 0 {
			t.Fatalf("expected do { ... } while (true) loop to skip forward-region prebinding")
		}
	})
}
