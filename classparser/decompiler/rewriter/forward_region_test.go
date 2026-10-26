package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func forwardRegionFixture() (*RewriteManager, []*core.Node) {
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	nodes := []*core.Node{
		core.NewNode(&statements.MiddleStatement{Flag: "start"}),
		core.NewNode(&statements.ConditionStatement{Condition: truth}),
		core.NewNode(&statements.ConditionStatement{Condition: truth}),
		core.NewNode(statements.NewExpressionStatement(truth)),
		core.NewNode(statements.NewExpressionStatement(truth)),
		core.NewNode(statements.NewExpressionStatement(truth)),
		core.NewNode(statements.NewExpressionStatement(truth)),
		core.NewNode(&statements.ReturnStatement{}),
	}
	for i, n := range nodes {
		n.Id, n.OriginPC, n.HasOriginPC = i, 10*i, true
	}
	root, owner, inner, first, second, work, exit, ret := nodes[0], nodes[1], nodes[2], nodes[3], nodes[4], nodes[5], nodes[6], nodes[7]
	root.AddNext(owner)
	owner.AddNext(first)
	owner.AddNext(inner)
	first.AddNext(work)
	inner.AddNext(second)
	inner.AddNext(exit)
	second.AddNext(work)
	work.AddNext(exit)
	exit.AddNext(ret)
	for _, n := range []*core.Node{owner, inner} {
		n := n
		n.TrueNode = func() *core.Node { return n.Next[0] }
		n.FalseNode = func() *core.Node { return n.Next[1] }
	}
	m := NewRootStatementManager(root)
	m.SetOriginalExceptionTable(nil)
	m.currentNodeId = 100
	m.IfNodes = []*core.Node{owner, inner}
	m.DominatorMap = GenerateDominatorTree(root)
	return m, nodes
}

func TestForwardRegionRetainsSharedWorkAndBypassPolarity(t *testing.T) {
	for _, swapped := range []bool{false, true} {
		for _, effectful := range []bool{false, true} {
			m, n := forwardRegionFixture()
			owner, inner, work, exit := n[1], n[2], n[5], n[6]
			from := inner
			if effectful {
				from = m.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(123, types.NewJavaPrimer(types.JavaInteger))))
				from.OriginPC, from.HasOriginPC = 25, true
				replaceNextInPlace(inner, exit, from)
				from.AddNext(exit)
			}
			if swapped {
				inner.Next[0], inner.Next[1] = inner.Next[1], inner.Next[0]
			}
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
			materializeForwardRegionExits(m, inner)
			region := m.forwardRegions[owner]
			if region == nil || region.exit != exit || region.label == "" {
				t.Fatal("closed partial decision has no exact exit binding")
			}
			index := 0
			if !effectful && !swapped {
				index = 1
			}
			jump := from.Next[index]
			transfer, ok := jump.Statement.(*statements.CustomStatement)
			if !ok {
				t.Fatalf("bypass became %T", jump.Statement)
			}
			kind, label, ok := transfer.SourceTransferProtocol()
			if !ok || kind != "break" || label != region.label || !jump.EncodedJumps[exit] || len(jump.Next) != 1 || jump.Next[0] != exit {
				t.Fatal("bypass destination or polarity changed")
			}
			materializeForwardRegionExits(m, inner)
			if from.Next[index] != jump {
				t.Fatal("second planning pass duplicated transfer")
			}
			if err := IfRewriter(m, inner); err != nil {
				t.Fatal(err)
			}
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
			originalWork := work.Statement
			if err := IfRewriter(m, owner); err != nil {
				t.Fatal(err)
			}
			wrapper := n[0].Next[0]
			block, ok := wrapper.Statement.(*statements.DoWhileStatement)
			if !ok || block.Label != region.label || len(wrapper.Next) != 1 || wrapper.Next[0] != exit || len(wrapper.EncodedJumps) != 0 {
				t.Fatal("region completion lost its ordinary continuation")
			}
			literal, ok := block.ConditionValue.(*values.JavaLiteral)
			if !ok || literal.Data != false {
				t.Fatal("forward region can repeat")
			}
			count := 0
			for _, st := range block.Body {
				if st == originalWork {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("shared work emitted %d times", count)
			}
		}
	}
}

func TestForwardRegionRejectsIncompleteOrEscapingGraphs(t *testing.T) {
	for _, name := range []string{"foreign entry", "cycle", "dead end", "competing terminal", "hidden edge", "catch entry", "protected wrapper", "unknown handler table", "different handler", "interior handler", "missing owner origin", "missing exit origin", "missing effect origin", "opaque leaf", "unqualified break", "unqualified continue", "opaque structured leaf", "too many nodes", "handler work limit"} {
		t.Run(name, func(t *testing.T) {
			m, n := forwardRegionFixture()
			root, owner, inner, work, exit := n[0], n[1], n[2], n[5], n[6]
			switch name {
			case "foreign entry":
				root.AddNext(work)
			case "cycle":
				work.AddNext(inner)
			case "dead end":
				work.RemoveAllNext()
			case "competing terminal":
				work.AddNext(n[7])
			case "hidden edge":
				work.HideNext = exit
			case "catch entry":
				work.IsCatchStart = true
			case "protected wrapper":
				work.HasProtectedRange = true
			case "unknown handler table":
				m.originalHandlersKnown = false
			case "different handler":
				m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 10, EndPc: 55, HandlerPc: 80, CatchType: 1}})
			case "interior handler":
				m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 45, EndPc: 55, HandlerPc: 80, CatchType: 1}})
			case "missing owner origin":
				owner.HasOriginPC = false
			case "missing exit origin":
				exit.HasOriginPC = false
			case "missing effect origin":
				work.HasOriginPC = false
			case "opaque leaf":
				work.Statement = &statements.CustomStatement{Name: "break"}
			case "unqualified break", "unqualified continue", "opaque structured leaf":
				var leaf statements.Statement = &statements.CustomStatement{}
				if name == "unqualified break" {
					leaf = statements.NewSourceTransferStatement("break", "")
				}
				if name == "unqualified continue" {
					leaf = statements.NewSourceTransferStatement("continue", "")
				}
				work.Statement = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{leaf}, nil)
			case "too many nodes":
				work.RemoveAllNext()
				cursor := work
				for i := 0; i < 1025; i++ {
					next := core.NewNode(statements.NewExpressionStatement(nil))
					next.HasOriginPC, next.OriginPC = true, 50
					cursor.AddNext(next)
					cursor = next
				}
				cursor.AddNext(exit)
			case "handler work limit":
				m.originalHandlers = make([]core.HandlerRange, 16384)
			}
			m.DominatorMap = GenerateDominatorTree(root)
			before := map[*core.Node][]*core.Node{}
			for _, node := range WalkNodeToList(root) {
				before[node] = slices.Clone(node.Next)
			}
			materializeForwardRegionExits(m, inner)
			if len(m.forwardRegions) != 0 {
				t.Fatal("unproved region accepted")
			}
			for node, next := range before {
				if !slices.Equal(node.Next, next) {
					t.Fatal("rejected proof mutated input graph")
				}
			}
		})
	}
}

func TestForwardBlockPreservesExistingNestedTransferOwner(t *testing.T) {
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	for _, kind := range []string{"break", "continue"} {
		for _, label := range []string{"", "EXISTING_REGION"} {
			jump := statements.NewSourceTransferStatement(kind, label)
			body := statements.NewIfStatement(truth, []statements.Statement{jump}, nil)
			remaining := 100
			if forwardBlockKeepsTransferBindings(body, 0, 0, &remaining) != (label != "") {
				t.Fatal("unqualified transfer changed owner")
			}
			remaining = 100
			if !forwardBlockKeepsTransferBindings(statements.NewDoWhileStatement(truth, []statements.Statement{body}), 0, 0, &remaining) {
				t.Fatal("inner loop lost its own transfer")
			}
		}
	}
	cyclic := statements.NewIfStatement(truth, nil, nil)
	cyclic.IfBody = []statements.Statement{cyclic}
	remaining := 1000
	if forwardBlockKeepsTransferBindings(cyclic, 0, 0, &remaining) {
		t.Fatal("cyclic statement tree accepted")
	}
}

func TestForwardRegionNormalizesSharedTerminalStoresFirst(t *testing.T) {
	m, n := forwardRegionFixture()
	owner, inner, shared, private, ret := n[1], n[2], n[3], n[4], n[7]
	integer := types.NewJavaPrimer(types.JavaInteger)
	local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
	for _, entry := range []struct {
		node *core.Node
		word int
	}{{shared, 1}, {private, 0}} {
		store := statements.NewAssignStatement(local, values.NewJavaLiteral(entry.word, integer), false)
		store.OriginPC, store.HasOriginPC = entry.node.OriginPC, true
		entry.node.Statement = store
		entry.node.RemoveAllNext()
		entry.node.AddNext(ret)
	}
	ret.Statement = &statements.ReturnStatement{JavaValue: local, OriginPC: ret.OriginPC, HasOriginPC: true}
	inner.RemoveAllNext()
	inner.AddNext(private)
	inner.AddNext(shared)
	owner.Next[0], owner.Next[1] = owner.Next[1], owner.Next[0]
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	if err := IfRewriter(m, inner); err != nil {
		t.Fatal(err)
	}
	if len(m.forwardRegions) != 0 {
		t.Fatal("shared inert terminal edge must be split before computing regional exits")
	}
	if len(shared.Source) != 1 || shared.Source[0] != owner {
		t.Fatal("edge split changed the outer arm's original assignment")
	}
}

func TestForwardRegionExitCanHaveExternalPredecessors(t *testing.T) {
	m, n := forwardRegionFixture()
	n[0].AddNext(n[6])
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	materializeForwardRegionExits(m, n[2])
	region := m.forwardRegions[n[1]]
	if region == nil || region.exit != n[6] {
		t.Fatal("outer bypass stole inner region ownership")
	}
	if len(n[0].Next) != 2 || n[0].Next[1] != n[6] {
		t.Fatal("changed external bypass")
	}
}

func TestForwardRegionExitTracksStructuredReplacement(t *testing.T) {
	m, n := forwardRegionFixture()
	exit := n[6]
	exit.Statement = &statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
	exit.RemoveAllNext()
	left, right := m.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)))), m.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))))
	left.OriginPC, left.HasOriginPC, right.OriginPC, right.HasOriginPC = 61, true, 62, true
	exit.AddNext(left)
	exit.AddNext(right)
	left.AddNext(n[7])
	right.AddNext(n[7])
	exit.TrueNode = func() *core.Node { return exit.Next[0] }
	exit.FalseNode = func() *core.Node { return exit.Next[1] }
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	materializeForwardRegionExits(m, n[2])
	region := m.forwardRegions[n[1]]
	if region == nil {
		t.Fatal("missing region")
	}
	if err := IfRewriter(m, exit); err != nil {
		t.Fatal(err)
	}
	if region.exit == exit {
		t.Fatal("pending region retained the replaced exit")
	}
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	if err := IfRewriter(m, n[2]); err != nil {
		t.Fatal(err)
	}
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	if err := IfRewriter(m, n[1]); err != nil {
		t.Fatal(err)
	}
	wrapper := n[0].Next[0]
	if len(wrapper.Next) != 1 || wrapper.Next[0] != region.exit {
		t.Fatal("region lost the structured continuation")
	}
	if len(m.forwardRegions) != 0 {
		t.Fatal("completed region remains pending")
	}
}
