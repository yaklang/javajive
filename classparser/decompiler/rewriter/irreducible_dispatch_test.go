package rewriter

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/graph"
)

func TestIrreducibleDispatchSyntheticNamesAreMethodLocal(t *testing.T) {
	manager := &RewriteManager{}
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	makeRegion := func(base int) (*core.Node, *core.Node, *core.Node) {
		a := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(newTestVariable(), nil, integer), values.NewJavaLiteral(base, integer), false))
		b := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
		exit := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(base, integer)))
		a.Id, b.Id, exit.Id = base, base+1, base+2
		a.AddNext(b)
		b.AddNext(exit)
		b.AddNext(a)
		b.JmpNode = a
		b.TrueNode = func() *core.Node { return a }
		b.FalseNode = func() *core.Node { return exit }
		condition := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
		condition.Id = base - 1
		condition.AddNext(a)
		condition.AddNext(b)
		condition.JmpNode = b
		condition.TrueNode = func() *core.Node { return b }
		condition.FalseNode = func() *core.Node { return a }
		return condition, a, b
	}

	firstRoot, firstA, firstB := makeRegion(10)
	manager.RootNode = firstRoot
	if !manager.lowerIrreducibleRegion(irreducibleDispatchRegion{
		component: dispatchTestSCC([]*core.Node{firstA, firstB}, []*core.Node{firstA, firstB}, firstRoot),
		exits:     []*core.Node{firstB.Next[0]},
	}) {
		t.Fatal(manager.irreducibleDispatchError)
	}
	first := renderDispatchTestGraph(manager.RootNode)

	secondRoot, secondA, secondB := makeRegion(20)
	manager.RootNode = secondRoot
	if !manager.lowerIrreducibleRegion(irreducibleDispatchRegion{
		component: dispatchTestSCC([]*core.Node{secondA, secondB}, []*core.Node{secondA, secondB}, secondRoot),
		exits:     []*core.Node{secondB.Next[0]},
	}) {
		t.Fatal(manager.irreducibleDispatchError)
	}
	second := renderDispatchTestGraph(manager.RootNode)
	if !strings.Contains(first, "jdecState0") || !strings.Contains(first, "jdecExit0") ||
		!strings.Contains(second, "jdecState1") || !strings.Contains(second, "jdecExit1") {
		t.Fatalf("synthetic names are not distinct:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestIrreducibleDispatchRequiresSemanticAdmission(t *testing.T) {
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	integer := types.NewJavaPrimer(types.JavaInteger)
	root := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
	a := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(newTestVariable(), nil, integer), values.NewJavaLiteral(1, integer), false))
	b := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
	exit := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(0, integer)))
	root.AddNext(a)
	root.AddNext(b)
	root.JmpNode = b
	root.TrueNode = func() *core.Node { return b }
	root.FalseNode = func() *core.Node { return a }
	a.AddNext(b)
	b.AddNext(exit)
	b.AddNext(a)
	b.JmpNode = a
	b.TrueNode = func() *core.Node { return a }
	b.FalseNode = func() *core.Node { return exit }
	manager := NewRootStatementManager(root)
	if manager.LowerIrreducibleDispatch(false, false) {
		t.Fatal("statement-only irreducibility bypassed semantic admission")
	}
	if manager.IrreducibleDispatchError() != "statement CFG lacks semantic irreducibility witness" {
		t.Fatalf("admission rejection reason = %q", manager.IrreducibleDispatchError())
	}
}

func TestIrreducibleDispatchRejectsConstructorsAndDeferredConditions(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	makeGraph := func(entryCallback, memberCallback func(values.JavaValue)) *core.Node {
		entryStatement := statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ)
		entryStatement.Callback = entryCallback
		entry := core.NewNode(entryStatement)
		a := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(newTestVariable(), nil, integer), values.NewJavaLiteral(1, integer), false))
		conditionStatement := statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ)
		conditionStatement.Callback = memberCallback
		b := core.NewNode(conditionStatement)
		exit := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(0, integer)))
		entry.Id, a.Id, b.Id, exit.Id = 1, 2, 3, 4
		entry.AddNext(a)
		entry.AddNext(b)
		entry.JmpNode = b
		entry.TrueNode = func() *core.Node { return b }
		entry.FalseNode = func() *core.Node { return a }
		a.AddNext(b)
		b.AddNext(exit)
		b.AddNext(a)
		b.JmpNode = a
		b.TrueNode = func() *core.Node { return a }
		b.FalseNode = func() *core.Node { return exit }
		return entry
	}

	constructorManager := NewRootStatementManager(makeGraph(nil, nil))
	if constructorManager.LowerIrreducibleDispatch(true, true) || constructorManager.IrreducibleDispatchError() != "constructor state dispatch is unsupported" {
		t.Fatalf("constructor dispatch was not rejected precisely: %q", constructorManager.IrreducibleDispatchError())
	}

	for _, test := range []struct {
		name       string
		entry      bool
		wantReason string
	}{
		{name: "member", wantReason: "cannot lower residual state node"},
		{name: "entry", entry: true, wantReason: "deferred condition callback"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			callback := func(values.JavaValue) { called = true }
			var entryCallback, memberCallback func(values.JavaValue)
			if test.entry {
				entryCallback = callback
			} else {
				memberCallback = callback
			}
			callbackManager := NewRootStatementManager(makeGraph(entryCallback, memberCallback))
			if callbackManager.LowerIrreducibleDispatch(true, false) {
				t.Fatal("callback-bearing condition was lowered")
			}
			if called {
				t.Fatal("dispatcher executed a deferred condition callback")
			}
			if !strings.Contains(callbackManager.IrreducibleDispatchError(), test.wantReason) {
				t.Fatalf("callback rejection reason = %q", callbackManager.IrreducibleDispatchError())
			}
		})
	}
}

func TestIrreducibleDispatchRedirectsSharedTerminalExit(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	exit := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(7, integer)))
	a := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(newTestVariable(), nil, integer), values.NewJavaLiteral(1, integer), false))
	b := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
	entry := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
	outside := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(newTestVariable(), nil, integer), values.NewJavaLiteral(2, integer), false))
	entry.Id, a.Id, b.Id, outside.Id, exit.Id = 1, 2, 3, 4, 5
	entry.AddNext(a)
	entry.AddNext(b)
	entry.JmpNode = b
	entry.TrueNode = func() *core.Node { return b }
	entry.FalseNode = func() *core.Node { return a }
	a.AddNext(b)
	b.AddNext(exit)
	b.AddNext(a)
	b.JmpNode = a
	b.TrueNode = func() *core.Node { return a }
	b.FalseNode = func() *core.Node { return exit }
	outside.AddNext(exit)
	exit.AddSource(outside)
	manager := NewRootStatementManager(entry)
	if !manager.lowerIrreducibleRegion(irreducibleDispatchRegion{
		component: dispatchTestSCC([]*core.Node{a, b}, []*core.Node{a, b}, entry),
		exits:     []*core.Node{exit},
	}) {
		t.Fatal(manager.IrreducibleDispatchError())
	}
	for _, next := range outside.Next {
		if next == exit {
			t.Fatal("outside predecessor still owns the original terminal exit")
		}
	}
	if len(outside.Next) != 1 || outside.Next[0].Statement == nil {
		t.Fatalf("outside predecessor was not redirected: %#v", outside.Next)
	}
}

func TestIrreducibleDispatchReanalyzesSequentialRegions(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	makeRegion := func(base int, exit *core.Node) (*core.Node, *core.Node, *core.Node) {
		a := core.NewNode(statements.NewAssignStatement(values.NewJavaRef(newTestVariable(), nil, integer), values.NewJavaLiteral(base, integer), false))
		b := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
		a.Id, b.Id = base, base+1
		a.AddNext(b)
		b.AddNext(exit)
		b.AddNext(a)
		b.JmpNode = a
		b.TrueNode = func() *core.Node { return a }
		b.FalseNode = func() *core.Node { return exit }
		return a, b, exit
	}

	terminal := core.NewNode(statements.NewReturnStatement(values.NewJavaLiteral(99, integer)))
	terminal.Id = 99
	secondA, secondB, _ := makeRegion(20, terminal)
	firstA, firstB, _ := makeRegion(30, secondA)

	pre := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
	pre.Id = 29
	pre.AddNext(firstA)
	pre.AddNext(firstB)
	pre.JmpNode = firstB
	pre.TrueNode = func() *core.Node { return firstB }
	pre.FalseNode = func() *core.Node { return firstA }

	root := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(true, boolean), values.NEQ))
	root.Id = 19
	root.AddNext(pre)
	root.AddNext(secondB)
	root.JmpNode = secondB
	root.TrueNode = func() *core.Node { return secondB }
	root.FalseNode = func() *core.Node { return pre }

	manager := NewRootStatementManager(root)
	if !manager.LowerIrreducibleDispatch(true, false) {
		t.Fatal(manager.IrreducibleDispatchError())
	}
	if !isReducibleCFG(manager.RootNode, GenerateDominatorTree(manager.RootNode)) {
		t.Fatal("sequential regions remain irreducible")
	}
	if manager.irreducibleDispatchCounter != 2 {
		t.Fatalf("dispatch count = %d, want 2", manager.irreducibleDispatchCounter)
	}
	rendered := renderDispatchTestGraph(manager.RootNode)
	for _, want := range []string{"jdecState0", "jdecState1", "return 99"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("sequential dispatch lost %q:\n%s", want, rendered)
		}
	}
}

func newTestVariable() *coreutils.VariableId {
	return coreutils.NewRootVariableId()
}

func dispatchTestSCC(nodes, entries []*core.Node, source *core.Node) graph.SCC[*core.Node] {
	edges := make([]graph.Edge[*core.Node], 0, len(entries))
	for _, entry := range entries {
		edges = append(edges, graph.Edge[*core.Node]{From: source, To: entry})
	}
	return graph.SCC[*core.Node]{Nodes: nodes, Cyclic: true, EntryTargets: entries, EntryEdges: edges}
}

func renderDispatchTestGraph(root *core.Node) string {
	manager := NewRootStatementManager(root)
	nodes, _ := manager.ToStatements(func(*core.Node) bool { return true })
	return statements.StatementsString(core.NodesToStatements(nodes), &class_context.ClassContext{})
}
