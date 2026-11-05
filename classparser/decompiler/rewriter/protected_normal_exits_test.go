package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestProtectedNormalExitsRequireCompleteOriginalBoundaries(t *testing.T) {
	for _, variant := range []string{"closed", "swapped", "foreign entry", "foreign returning entry", "shared returning tail", "direct method exit", "missing handler table", "missing handler PC", "wrong handler PC", "wrong interval", "physical catchall", "catchall", "fallthrough handler", "hidden owner", "hidden branch", "encoded branch", "third arm", "unknown returning effect", "protected alternate effect", "missing return PC", "returning cycle", "alternate reentry", "unknown prefix", "unsealed prefix exit", "oversized handler table"} {
		t.Run(variant, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{})
			owner := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			owner.HasProtectedRange = true
			owner.ProtectedStartPC, owner.ProtectedEndPC = 10, 30
			truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			condition := core.NewNode(&statements.ConditionStatement{Condition: truth})
			condition.HasOriginPC, condition.OriginPC = true, 20
			local := layerTestRef("java.lang.Object")
			store := core.NewNode(statements.NewAssignStatement(local, values.JavaNull, false))
			store.HasOriginPC, store.OriginPC = true, 25
			commit := core.NewNode(statements.NewExpressionStatement(layerTestCall(30)))
			commit.HasOriginPC, commit.OriginPC = true, 30
			ret := &statements.ReturnStatement{JavaValue: local, HasOriginPC: true, OriginPC: 40}
			returned := core.NewNode(ret)
			returned.HasOriginPC, returned.OriginPC = true, 40
			alternate := core.NewNode(statements.NewExpressionStatement(layerTestCall(50)))
			alternate.HasOriginPC, alternate.OriginPC = true, 50
			after := core.NewNode(&statements.ReturnStatement{JavaValue: values.JavaNull, HasOriginPC: true, OriginPC: 60})
			after.HasOriginPC, after.OriginPC = true, 60
			handler := core.NewNode(&statements.ReturnStatement{JavaValue: values.JavaNull, HasOriginPC: true, OriginPC: 80})
			handler.HasOriginPC, handler.OriginPC = true, 80
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 80, ProtectedRanges: [][2]int{{10, 30}}}
			root.AddNext(owner)
			owner.AddNext(condition)
			owner.AddNext(handler)
			condition.AddNext(store)
			condition.AddNext(alternate)
			store.AddNext(commit)
			commit.AddNext(returned)
			alternate.AddNext(after)
			owner.ProtectedEnd = commit
			condition.TrueNode = func() *core.Node { return condition.Next[0] }
			condition.FalseNode = func() *core.Node { return condition.Next[1] }
			m := NewRootStatementManager(root)
			m.TryNodes = []*core.Node{owner}
			m.currentNodeId = 100
			m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 10, EndPc: 30, HandlerPc: 80, CatchType: 1}})
			want := variant == "closed" || variant == "swapped"
			switch variant {
			case "swapped":
				condition.Next[0], condition.Next[1] = condition.Next[1], condition.Next[0]
			case "foreign entry":
				root.AddNext(store)
			case "foreign returning entry":
				root.AddNext(commit)
			case "shared returning tail":
				alternate.ReplaceNext(after, returned)
			case "direct method exit":
				store.ReplaceNext(commit, returned)
				owner.ProtectedEnd = returned
			case "missing handler table":
				m.originalHandlersKnown = false
			case "missing handler PC":
				handler.HasOriginPC = false
			case "wrong handler PC":
				handler.CatchHandler.EntryPC++
			case "wrong interval":
				handler.CatchHandler.ProtectedRanges[0][1]++
			case "physical catchall":
				m.originalHandlers[0].CatchType = 0
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "fallthrough handler":
				handler.AddNext(after)
			case "hidden owner":
				owner.HideNext = after
			case "hidden branch":
				condition.HideNext = after
			case "encoded branch":
				condition.EncodedJumps = map[*core.Node]bool{alternate: true}
			case "third arm":
				condition.AddNext(after)
			case "unknown returning effect":
				commit.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "protected alternate effect":
				alternate.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 29
			case "missing return PC":
				ret.HasOriginPC = false
			case "returning cycle":
				commit.ReplaceNext(returned, commit)
			case "alternate reentry":
				alternate.ReplaceNext(after, store)
			case "unknown prefix":
				prefix := core.NewNode(&statements.CustomStatement{})
				owner.ReplaceNext(condition, prefix)
				prefix.AddNext(condition)
			case "unsealed prefix exit":
				prefix := core.NewNode(&statements.ConditionStatement{Condition: truth})
				prefix.HasOriginPC, prefix.OriginPC = true, 11
				owner.ReplaceNext(condition, prefix)
				prefix.AddNext(condition)
				prefix.AddNext(alternate)
			case "oversized handler table":
				m.originalHandlers = make([]core.HandlerRange, 513)
			}
			m.DominatorMap = GenerateDominatorTree(root)
			materializeProtectedNormalExit(m, condition)
			region := m.forwardRegions[owner]
			if (region != nil) != want {
				t.Fatalf("planned=%v want=%v", region != nil, want)
			}
			if !want {
				return
			}
			if region.exit != alternate || region.protectedEnd != commit || region.label == "" || store.Next[0] != commit || commit.Next[0] != returned || owner.Next[1] != handler {
				t.Fatal("changed original computation, handler, or return")
			}
			index := 1
			if variant == "swapped" {
				index = 0
			}
			jump := condition.Next[index]
			leaf, ok := jump.Statement.(*statements.CustomStatement)
			if !ok {
				t.Fatal("missing selected transfer")
			}
			kind, label, known := leaf.SourceTransferProtocol()
			if !known || kind != "break" || label != region.label || len(jump.Next) != 1 || jump.Next[0] != alternate || !jump.EncodedJumps[alternate] {
				t.Fatal("lost selected alternate boundary")
			}
			materializeProtectedNormalExit(m, condition)
			if condition.Next[index] != jump {
				t.Fatal("repeated planning duplicated the transfer")
			}
		})
	}
}

func TestIfCollectionDoesNotConsumeDominatedEncodedContinuation(t *testing.T) {
	root := core.NewNode(&statements.MiddleStatement{})
	condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
	jump := core.NewNode(statements.NewSourceTransferStatement("break", "outer"))
	effect := core.NewNode(statements.NewExpressionStatement(layerTestCall(30)))
	after := core.NewNode(&statements.ReturnStatement{})
	other := core.NewNode(&statements.ReturnStatement{})
	root.AddNext(condition)
	condition.AddNext(jump)
	condition.AddNext(other)
	jump.AddNext(effect)
	effect.AddNext(after)
	jump.EncodedJumps = map[*core.Node]bool{effect: true}
	condition.TrueNode = func() *core.Node { return condition.Next[0] }
	condition.FalseNode = func() *core.Node { return condition.Next[1] }
	m := NewRootStatementManager(root)
	m.SetOriginalExceptionTable(nil)
	m.DominatorMap = GenerateDominatorTree(root)
	if err := IfRewriter(m, condition); err != nil {
		t.Fatal(err)
	}
	result := root.Next[0]
	branch := result.Statement.(*statements.IfStatement)
	if len(branch.IfBody) != 1 || branch.IfBody[0] != jump.Statement || len(branch.ElseBody) != 1 || branch.ElseBody[0] != other.Statement || !result.EncodedJumps[effect] {
		t.Fatal("collected execution beyond an already emitted break")
	}
}

func TestTryCollectionDoesNotConsumeDominatedEncodedContinuation(t *testing.T) {
	root := core.NewNode(&statements.MiddleStatement{})
	owner := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
	jump := core.NewNode(statements.NewSourceTransferStatement("break", "outer"))
	effect := core.NewNode(statements.NewExpressionStatement(layerTestCall(30)))
	after := core.NewNode(&statements.ReturnStatement{})
	handler := core.NewNode(t10ExceptionAssign("failure"))
	handler.IsCatchStart = true
	handler.AddNext(core.NewNode(&statements.ReturnStatement{}))
	root.AddNext(owner)
	owner.AddNext(jump)
	owner.AddNext(handler)
	jump.AddNext(effect)
	effect.AddNext(after)
	jump.EncodedJumps = map[*core.Node]bool{effect: true}
	m := NewRootStatementManager(root)
	m.DominatorMap = GenerateDominatorTree(root)
	if err := TryRewriter(m, owner); err != nil {
		t.Fatal(err)
	}
	result := root.Next[0]
	structured := result.Statement.(*statements.TryCatchStatement)
	if len(structured.TryBody) != 1 || structured.TryBody[0] != jump.Statement || !result.EncodedJumps[effect] {
		t.Fatal("try body consumed effects after an emitted break")
	}
}

func TestProtectedPrefixRequiresOriginalLoopTransferOwnership(t *testing.T) {
	for _, variant := range []string{"continue", "break", "qualified", "unrecorded", "wrong target", "wrong kind", "wrong label", "unsealed", "foreign loop", "unowned container", "deep statement", "cyclic statement"} {
		t.Run(variant, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{})
			owner := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			loop := core.NewNode(statements.NewDoWhileStatement(truth, nil))
			leaf := statements.NewSourceTransferStatement("continue", "")
			jump := core.NewNode(leaf)
			branch := statements.NewIfStatement(truth, []statements.Statement{leaf}, nil)
			container := core.NewNode(branch)
			root.AddNext(owner)
			owner.AddNext(loop)
			loop.AddNext(container)
			m := NewRootStatementManager(root)
			m.WhileNode = []*core.Node{loop}
			m.recordLoopTransfer(jump, loop, "continue")
			want := variant == "continue" || variant == "break" || variant == "qualified"
			switch variant {
			case "break":
				leaf.RetargetSourceTransfer("break", "")
				m.recordLoopTransfer(jump, loop, "break")
			case "qualified":
				loop.Statement.(*statements.DoWhileStatement).Label = "scope"
				leaf.RetargetSourceTransfer("continue", "scope")
			case "unrecorded":
				delete(m.loopTransfers, jump)
			case "wrong target":
				m.recordLoopTransfer(jump, owner, "continue")
			case "wrong kind":
				leaf.RetargetSourceTransfer("break", "")
			case "wrong label":
				leaf.RetargetSourceTransfer("continue", "unrelated")
			case "unsealed":
				branch.IfBody[0] = &statements.CustomStatement{LoopTransferKind: "continue", Name: "continue"}
			case "foreign loop":
				root.AddNext(loop)
			case "unowned container":
				owner.AddNext(container)
			case "deep statement":
				for i := 0; i < 30; i++ {
					branch = statements.NewIfStatement(truth, []statements.Statement{branch}, nil)
				}
				container.Statement = branch
			case "cyclic statement":
				branch.IfBody = []statements.Statement{branch}
			}
			m.DominatorMap = GenerateDominatorTree(root)
			if got := protectedNormalNodeCovered(m, owner, container, func(pc int) bool { return pc >= 10 && pc < 30 }); got != want {
				t.Fatalf("owned loop transfer=%t want=%t", got, want)
			}
		})
	}
}
