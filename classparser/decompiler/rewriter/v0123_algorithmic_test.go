package rewriter

import (
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestASTObjectSafetyIdentityAnalysis(t *testing.T) {
	ctx := &class_context.ClassContext{}
	idSafe := utils.NewRootVariableId()
	idSafe.SetName("x")
	idShadowUnsafe := utils.NewRootVariableId()
	idShadowUnsafe.SetName("x")

	refSafe := values.NewJavaRef(idSafe, nil, types.NewJavaClass("java.lang.Object"))
	refShadow := values.NewJavaRef(idShadowUnsafe, nil, types.NewJavaClass("java.lang.String"))

	listRef := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.List"))
	listRef.Id.SetName("list")

	containsCall := &values.FunctionCallExpression{
		Kind:         values.InvokeInterface,
		ClassName:    "java.util.List",
		FunctionName: "contains",
		Descriptor:   "(Ljava/lang/Object;)Z",
		Object:       listRef,
		Arguments:    []values.JavaValue{refSafe},
	}
	strConsumerCall := &values.FunctionCallExpression{
		Kind:         values.InvokeStatic,
		ClassName:    "pkg.Helper",
		FunctionName: "consumeString",
		Descriptor:   "(Ljava/lang/String;)V",
		IsStatic:     true,
		Arguments:    []values.JavaValue{refShadow},
	}

	sts := []statements.Statement{
		statements.NewExpressionStatement(containsCall),
		statements.NewExpressionStatement(strConsumerCall),
	}

	if !identityUsesAreObjectSafe(sts, idSafe, ctx, true) {
		t.Fatal("expected idSafe used only as Object formal arg to remain Object-safe even when same-named idShadowUnsafe requires String")
	}
	if identityUsesAreObjectSafe(sts, idShadowUnsafe, ctx, true) {
		t.Fatal("expected idShadowUnsafe passed to consumeString(String) NOT to be Object-safe")
	}

	// Null-only concrete local reconciliation via AST VariableId identity.
	idNullOnly := utils.NewRootVariableId()
	idNullOnly.SetName("var85")
	declNull := values.NewJavaRef(idNullOnly, nil, types.NewJavaClass("java.lang.Integer"))
	useClassArr := values.NewJavaRef(idNullOnly, nil, types.NewJavaClass("java.lang.Integer"))
	nullAssign := statements.NewAssignStatement(declNull, values.JavaNull, true)
	ctorCall := &values.FunctionCallExpression{
		Kind:         values.InvokeVirtual,
		ClassName:    "java.lang.Class",
		FunctionName: "getDeclaredConstructor",
		Descriptor:   "([Ljava/lang/Class;)Ljava/lang/reflect/Constructor;",
		Object:       values.NewJavaClassValue(types.NewJavaClass("pkg.Dummy")),
		Arguments:    []values.JavaValue{useClassArr},
	}
	reconcileList := []statements.Statement{nullAssign, statements.NewExpressionStatement(ctorCall)}
	reconcileNullOnlyConcreteLocals(&reconcileList, ctx)
	if got := declNull.Type().String(ctx); got != "Class[]" {
		t.Fatalf("expected null-only local reconciled to Class[], got %q", got)
	}
}

func TestLoopRewriterExcludesInteriorJoinInCircleSet(t *testing.T) {
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	start := core.NewNode(&statements.MiddleStatement{Flag: "start"})
	loopWrapper := core.NewNode(statements.NewDoWhileStatement(truth, nil))
	loopWrapper.Statement.(*statements.DoWhileStatement).Label = "LOOP_1"
	condInner := core.NewNode(&statements.ConditionStatement{Condition: truth})
	condInner.TrueNode = func() *core.Node { return condInner.Next[0] }
	condInner.FalseNode = func() *core.Node { return condInner.Next[1] }
	armA := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
	armB := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
	// Interior join reachable from both armA and armB (neither dominates join), jumping back to loopWrapper.
	join := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))))

	for i, n := range []*core.Node{start, loopWrapper, condInner, armA, armB, join} {
		n.Id = i + 1
	}
	start.AddNext(loopWrapper)
	loopWrapper.AddNext(condInner)
	condInner.AddNext(armA)
	condInner.AddNext(armB)
	armA.AddNext(join)
	armB.AddNext(join)
	join.AddNext(loopWrapper)

	m := NewRootStatementManager(start)
	m.currentNodeId = 50
	m.DominatorMap = GenerateDominatorTree(start)
	m.WhileNode = []*core.Node{loopWrapper}

	if err := LoopRewriter(m, loopWrapper); err != nil {
		t.Fatalf("LoopRewriter failed on interior join: %v", err)
	}
	structured := start.Next[0]
	for _, nx := range structured.Next {
		if nx == join || nx == loopWrapper || nx == structured {
			t.Fatalf("loopWrapper.Next leaked interior circle node id=%d (%T)", nx.Id, nx.Statement)
		}
	}
	if _, err := m.ToStatementsFromNode(start, nil); err != nil {
		t.Fatalf("ToStatementsFromNode failed after LoopRewriter (expected no ErrHasCircle): %v", err)
	}
}

func TestPreStructureInnerTryInSharedCatchHandler(t *testing.T) {
	ctx := &class_context.ClassContext{}
	excCV := func() *values.CustomValue {
		return &values.CustomValue{
			Flag:       "exception",
			TypeFunc:   func() types.JavaType { return types.NewJavaClass("java.lang.Throwable") },
			StringFunc: func(_ *class_context.ClassContext) string { return "Exception" },
		}
	}
	start := core.NewNode(&statements.MiddleStatement{Flag: "start"})
	try1 := core.NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
	try1.HasOriginPC, try1.OriginPC = true, 10
	try2 := core.NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
	try2.HasOriginPC, try2.OriginPC = true, 20
	body2 := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(42, types.NewJavaPrimer(types.JavaInteger))))
	ret := core.NewNode(statements.NewReturnStatement(nil))
	end := core.NewNode(&statements.MiddleStatement{Flag: "end"})

	sharedCatchID := utils.NewRootVariableId()
	sharedCatchID.SetName("exOuter")
	sharedCatch := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(sharedCatchID, nil, types.NewJavaClass("java.lang.Throwable")), excCV(), true))
	sharedCatch.IsCatchStart = true
	sharedCatch.HasOriginPC, sharedCatch.OriginPC = true, 40

	try3 := core.NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
	try3.HasOriginPC, try3.OriginPC = true, 50
	try3Body := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))))
	innerCatchID := utils.NewRootVariableId()
	innerCatchID.SetName("exInner")
	innerCatch := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(innerCatchID, nil, types.NewJavaClass("java.lang.Throwable")), excCV(), true))
	innerCatch.IsCatchStart = true
	innerCatch.HasOriginPC, innerCatch.OriginPC = true, 70

	for i, n := range []*core.Node{start, try1, try2, body2, ret, end, sharedCatch, try3, try3Body, innerCatch} {
		n.Id = i + 1
	}
	start.AddNext(try1)
	try1.AddNext(try2)
	try1.AddNext(sharedCatch)
	try2.AddNext(body2)
	try2.AddNext(sharedCatch)
	body2.AddNext(ret)
	ret.AddNext(end)
	sharedCatch.AddNext(try3)
	try3.AddNext(try3Body)
	try3.AddNext(innerCatch)
	try3Body.AddNext(ret)
	innerCatch.AddNext(ret)

	m := NewRootStatementManager(start)
	m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{
		{StartPc: 10, EndPc: 40, HandlerPc: 40, CatchType: 1},
		{StartPc: 20, EndPc: 40, HandlerPc: 40, CatchType: 1},
		{StartPc: 50, EndPc: 70, HandlerPc: 70, CatchType: 1},
	})
	if err := m.Rewrite(); err != nil {
		t.Fatalf("Rewrite failed on shared catch with nested TryNode: %v", err)
	}
	nodesOut, err := m.ToStatementsFromNode(m.RootNode, nil)
	if err != nil {
		t.Fatalf("ToStatementsFromNode failed: %v", err)
	}
	var rendered strings.Builder
	for _, n := range nodesOut {
		if n != nil && n.Statement != nil {
			rendered.WriteString(n.Statement.String(ctx))
			rendered.WriteByte('\n')
		}
	}
	out := rendered.String()
	if strings.Contains(out, "tryStart") || strings.Contains(out, "Exception") {
		t.Fatalf("leaked tryStart or raw caught-exception sentinel into structured body:\n%s", out)
	}
}

func TestForwardRegionAbruptSwitchAndMultiTerminatorSealing(t *testing.T) {
	t.Run("forwardStructuredAbruptMethodExit_switch_and_try", func(t *testing.T) {
		val := values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
		abruptRet := statements.NewReturnStatement(values.NewJavaLiteral(9, types.NewJavaPrimer(types.JavaInteger)))
		abruptRet.HasOriginPC = true
		swAbrupt := statements.NewSwitchStatement(val, []*statements.CaseItem{
			{IntValue: 1, Body: []statements.Statement{abruptRet}},
			{IsDefault: true, Body: []statements.Statement{abruptRet}},
		})
		if !forwardStructuredAbruptMethodExit(swAbrupt, 0) {
			t.Fatal("expected all-abrupt switch with default to satisfy forwardStructuredAbruptMethodExit")
		}
		swMissingDefault := statements.NewSwitchStatement(val, []*statements.CaseItem{
			{IntValue: 1, Body: []statements.Statement{abruptRet}},
		})
		if forwardStructuredAbruptMethodExit(swMissingDefault, 0) {
			t.Fatal("expected switch without default NOT to satisfy forwardStructuredAbruptMethodExit")
		}
		tcAbrupt := statements.NewTryCatchStatement([]statements.Statement{abruptRet}, [][]statements.Statement{{abruptRet}})
		if !forwardStructuredAbruptMethodExit(tcAbrupt, 0) {
			t.Fatal("expected all-abrupt TryCatchStatement to satisfy forwardStructuredAbruptMethodExit")
		}
	})

	t.Run("seal_owned_multi_terminator_forward_region_exits", func(t *testing.T) {
		truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
		owner := core.NewNode(&statements.ConditionStatement{Condition: truth})
		leftBranch := core.NewNode(&statements.ConditionStatement{Condition: truth})
		rightBranch := core.NewNode(&statements.ConditionStatement{Condition: truth})
		lWork := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		rWork := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))))
		retA := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(10, types.NewJavaPrimer(types.JavaInteger))))
		retB := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(20, types.NewJavaPrimer(types.JavaInteger))))
		end := core.NewNode(&statements.MiddleStatement{Flag: "end"})

		nodes := []*core.Node{root, owner, leftBranch, rightBranch, lWork, rWork, retA, retB, end}
		for i, n := range nodes {
			n.Id = i + 1
			if n != root && n != end {
				n.HasOriginPC = true
				n.OriginPC = (i + 1) * 10
			}
		}
		retA.Statement.(*statements.ReturnStatement).HasOriginPC = true
		retA.Statement.(*statements.ReturnStatement).OriginPC = retA.OriginPC
		retB.Statement.(*statements.ReturnStatement).HasOriginPC = true
		retB.Statement.(*statements.ReturnStatement).OriginPC = retB.OriginPC

		root.AddNext(owner)
		owner.AddNext(leftBranch)
		owner.AddNext(rightBranch)
		leftBranch.AddNext(lWork)
		leftBranch.AddNext(retA)
		rightBranch.AddNext(rWork)
		rightBranch.AddNext(retB)
		lWork.AddNext(retB)
		rWork.AddNext(retA)
		retA.AddNext(end)
		retB.AddNext(end)
		for _, n := range []*core.Node{owner, leftBranch, rightBranch} {
			n := n
			n.TrueNode = func() *core.Node { return n.Next[0] }
			n.FalseNode = func() *core.Node { return n.Next[1] }
		}

		m := NewRootStatementManager(root)
		m.SetOriginalExceptionTable(nil)
		m.currentNodeId = 100
		m.IfNodes = []*core.Node{owner, leftBranch, rightBranch}
		m.DominatorMap = GenerateDominatorTree(root)

		for _, cond := range []*core.Node{leftBranch, rightBranch} {
			materializeForwardRegionExits(m, cond)
			if err := IfRewriter(m, cond); err != nil {
				t.Fatalf("inner IfRewriter(%d) failed: %v", cond.Id, err)
			}
			m.DominatorMap = GenerateDominatorTree(root)
		}
		if err := IfRewriter(m, owner); err != nil {
			t.Fatalf("outer IfRewriter failed with multi-terminator forward regions: %v", err)
		}
		if slices.Contains(m.IfNodes, nil) {
			t.Fatal("unexpected nil entry in IfNodes")
		}
	})
}
