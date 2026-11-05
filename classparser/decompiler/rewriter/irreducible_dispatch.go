package rewriter

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/graph"
)

type irreducibleDispatchRegion struct {
	component graph.SCC[*core.Node]
	exits     []*core.Node
}

// LowerIrreducibleDispatch converts supported normal-flow irreducible kernels
// before ordinary if/loop/switch structuring consumes their edge identity.
func (m *RewriteManager) LowerIrreducibleDispatch(admitted, constructor bool) bool {
	if m == nil || m.RootNode == nil || isReducibleCFG(m.RootNode, GenerateDominatorTree(m.RootNode)) {
		return true
	}
	if !admitted {
		m.irreducibleDispatchError = "statement CFG lacks semantic irreducibility witness"
		return false
	}
	if constructor {
		m.irreducibleDispatchError = "constructor state dispatch is unsupported"
		return false
	}
	maxRewrites := len(reachableGraphNodes(m.RootNode))
	for rewrites := 0; rewrites < maxRewrites; rewrites++ {
		region, found := m.nextIrreducibleDispatchRegion()
		if !found {
			if isReducibleCFG(m.RootNode, GenerateDominatorTree(m.RootNode)) {
				return true
			}
			if m.irreducibleDispatchError == "" {
				m.irreducibleDispatchError = "no residual multi-entry SCC"
			}
			return false
		}
		if !m.lowerIrreducibleRegion(region) {
			if m.irreducibleDispatchError == "" {
				m.irreducibleDispatchError = "residual region rejected by dispatcher"
			}
			return false
		}
	}
	m.irreducibleDispatchError = "dispatcher rewrite limit exceeded"
	return false
}

func reachableGraphNodes(root *core.Node) []*core.Node {
	nodes := []*core.Node{}
	_ = core.WalkGraph(root, func(node *core.Node) ([]*core.Node, error) {
		nodes = append(nodes, node)
		return node.Next, nil
	})
	return nodes
}

func (m *RewriteManager) nextIrreducibleDispatchRegion() (irreducibleDispatchRegion, bool) {
	nodes := reachableGraphNodes(m.RootNode)
	dominators := GenerateDominatorTree(m.RootNode)
	forward := func(node *core.Node) []*core.Node {
		out := make([]*core.Node, 0, len(node.Next))
		for _, next := range node.Next {
			if !dominatesNode(dominators, next, node) {
				out = append(out, next)
			}
		}
		return out
	}
	fullAnalysis := graph.Analyze(nodes, []*core.Node{m.RootNode}, func(node *core.Node) []*core.Node { return node.Next })
	analysis := graph.Analyze(nodes, []*core.Node{m.RootNode}, forward)
	regions := make([]irreducibleDispatchRegion, 0, len(analysis.CyclicSCCs))
	selected := map[*core.Node]bool{}
	for _, component := range analysis.CyclicSCCs {
		if len(component.EntryTargets) < 2 {
			continue
		}
		for _, full := range fullAnalysis.CyclicSCCs {
			fullSet := make(map[*core.Node]bool, len(full.Nodes))
			for _, node := range full.Nodes {
				fullSet[node] = true
			}
			contains := true
			for _, node := range component.Nodes {
				contains = contains && fullSet[node]
			}
			if contains {
				component = full
				break
			}
		}
		if len(component.Nodes) == 0 || selected[component.Nodes[0]] {
			continue
		}
		selected[component.Nodes[0]] = true
		componentSet := make(map[*core.Node]bool, len(component.Nodes))
		for _, node := range component.Nodes {
			componentSet[node] = true
		}
		exitSet := map[*core.Node]bool{}
		for _, node := range component.Nodes {
			for _, next := range node.Next {
				if !componentSet[next] {
					exitSet[next] = true
				}
			}
		}
		if len(exitSet) == 0 {
			m.irreducibleDispatchError = "residual region has no exit"
			return irreducibleDispatchRegion{}, false
		}
		exits := make([]*core.Node, 0, len(exitSet))
		for node := range exitSet {
			exits = append(exits, node)
		}
		sort.SliceStable(exits, func(i, j int) bool { return exits[i].Id < exits[j].Id })
		regions = append(regions, irreducibleDispatchRegion{component: component, exits: exits})
	}
	if len(regions) == 0 {
		return irreducibleDispatchRegion{}, false
	}
	sort.SliceStable(regions, func(i, j int) bool {
		return regions[i].component.Nodes[0].Id > regions[j].component.Nodes[0].Id
	})
	return regions[0], true
}

func dominatesNode(tree map[*core.Node][]*core.Node, dominator, node *core.Node) bool {
	if dominator == nil || node == nil {
		return false
	}
	if dominator == node {
		return true
	}
	pending := append([]*core.Node(nil), tree[dominator]...)
	for len(pending) > 0 {
		last := len(pending) - 1
		current := pending[last]
		pending = pending[:last]
		if current == node {
			return true
		}
		pending = append(pending, tree[current]...)
	}
	return false
}

func (m *RewriteManager) lowerIrreducibleRegion(region irreducibleDispatchRegion) bool {
	members := map[*core.Node]bool{}
	for _, node := range region.component.Nodes {
		if node == nil {
			m.irreducibleDispatchError = "residual region contains nil node"
			return false
		}
		if node.IsTryCatch || node.IsCatchStart || node.HasProtectedRange || node.HideNext != nil || len(node.EncodedJumps) != 0 {
			m.irreducibleDispatchError = "residual region has unsupported node metadata"
			return false
		}
		members[node] = true
	}
	for _, edge := range region.component.EntryEdges {
		if members[edge.From] || !members[edge.To] {
			continue
		}
		if condition, ok := edge.From.Statement.(*statements.ConditionStatement); ok && condition.Callback != nil {
			m.irreducibleDispatchError = fmt.Sprintf("residual entry source id=%d has deferred condition callback", edge.From.Id)
			return false
		}
	}
	for _, exit := range region.exits {
		for _, source := range exit.Source {
			if members[source] {
				continue
			}
			if condition, ok := source.Statement.(*statements.ConditionStatement); ok && condition.Callback != nil {
				m.irreducibleDispatchError = fmt.Sprintf("outside exit source id=%d has deferred condition callback", source.Id)
				return false
			}
		}
	}
	entries := append([]*core.Node(nil), region.component.EntryTargets...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Id < entries[j].Id })
	stateOf := map[*core.Node]int{}
	for index, entry := range entries {
		stateOf[entry] = index
	}
	remaining := append([]*core.Node(nil), region.component.Nodes...)
	sort.SliceStable(remaining, func(i, j int) bool { return remaining[i].Id < remaining[j].Id })
	for _, node := range remaining {
		if _, exists := stateOf[node]; !exists {
			stateOf[node] = len(stateOf)
		}
	}

	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	dispatchID := m.irreducibleDispatchCounter
	m.irreducibleDispatchCounter++
	stateID := coreutils.NewRootVariableId()
	stateID.SetName(fmt.Sprintf("jdecState%d", dispatchID))
	state := values.NewJavaRef(stateID, nil, integer)
	exitID := coreutils.NewRootVariableId()
	exitID.SetName(fmt.Sprintf("jdecExit%d", dispatchID))
	exitState := values.NewJavaRef(exitID, nil, integer)
	exitOf := make(map[*core.Node]int, len(region.exits))
	for index, exit := range region.exits {
		exitOf[exit] = index
	}
	label := m.NewLoopLabel()
	cases := make([]*statements.CaseItem, 0, len(remaining))
	for _, node := range remaining {
		body, ok := dispatchCaseBody(node, members, stateOf, state, exitOf, exitState, label)
		if !ok {
			m.irreducibleDispatchError = fmt.Sprintf("cannot lower residual state node id=%d statement=%T next=%d", node.Id, node.Statement, len(node.Next))
			return false
		}
		cases = append(cases, statements.NewCaseItem(stateOf[node], body))
	}
	switchStatement := statements.NewSwitchStatement(state, cases)
	loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolean), []statements.Statement{switchStatement})
	loop.Label = label
	loopNode := m.NewNode(loop)
	dispatchRoot := buildExitDispatch(m, exitState, region.exits, exitOf)
	if dispatchRoot == nil {
		if m.irreducibleDispatchError == "" {
			m.irreducibleDispatchError = "cannot build exit dispatch"
		}
		return false
	}
	loopNode.AddNext(dispatchRoot)
	for _, exit := range region.exits {
		for _, source := range append([]*core.Node(nil), exit.Source...) {
			if members[source] {
				continue
			}
			assign := m.NewNode(statements.NewAssignStatement(exitState, values.NewJavaLiteral(exitOf[exit], integer), false))
			assign.AddNext(dispatchRoot)
			replaceNextInPlace(source, exit, assign)
		}
	}

	for _, edge := range region.component.EntryEdges {
		if members[edge.From] || !members[edge.To] {
			continue
		}
		assign := m.NewNode(statements.NewAssignStatement(state, values.NewJavaLiteral(stateOf[edge.To], integer), false))
		assign.AddNext(loopNode)
		replaceNextInPlace(edge.From, edge.To, assign)
	}
	originalRoot := m.RootNode
	initial := 0
	if members[originalRoot] {
		initial = stateOf[originalRoot]
		m.RootNode = loopNode
	}
	oldRoot := m.RootNode
	declareState := m.NewNode(statements.NewAssignStatement(state, values.NewJavaLiteral(initial, integer), true))
	declareExit := m.NewNode(statements.NewAssignStatement(exitState, values.NewJavaLiteral(0, integer), true))
	declareState.AddNext(declareExit)
	declareExit.AddNext(oldRoot)
	m.RootNode = declareState
	for _, node := range remaining {
		node.RemoveAllSource()
		node.RemoveAllNext()
	}
	return true
}

func buildExitDispatch(m *RewriteManager, exitState *values.JavaRef, exits []*core.Node, exitOf map[*core.Node]int) *core.Node {
	if len(exits) == 1 {
		return exits[0]
	}
	items := make([]*statements.CaseItem, 0, len(exits)+1)
	owned := map[*core.Node]bool{}
	for _, exit := range exits {
		body, ok := linearTerminalExit(exit, owned)
		if !ok {
			if exit == nil {
				m.irreducibleDispatchError = "nil terminal exit"
			} else {
				m.irreducibleDispatchError = fmt.Sprintf("nonlinear terminal exit id=%d statement=%T next=%d", exit.Id, exit.Statement, len(exit.Next))
			}
			return nil
		}
		items = append(items, statements.NewCaseItem(exitOf[exit], body))
	}
	def := statements.NewCaseItem(0, []statements.Statement{statements.NewThrowStatement(values.JavaNull)})
	def.IsDefault = true
	items = append(items, def)
	return m.NewNode(statements.NewSwitchStatement(exitState, items))
}

func linearTerminalExit(start *core.Node, owned map[*core.Node]bool) ([]statements.Statement, bool) {
	body := []statements.Statement{}
	seen := map[*core.Node]bool{}
	for node := start; node != nil; {
		if seen[node] || owned[node] || node.Statement == nil || len(body) >= 64 || node.IsTryCatch || node.IsCatchStart || node.HasProtectedRange {
			return nil, false
		}
		if node == start {
			if len(node.Source) == 0 {
				return nil, false
			}
		} else if len(node.Source) != 1 {
			return nil, false
		}
		seen[node] = true
		owned[node] = true
		if _, middle := node.Statement.(*statements.MiddleStatement); !middle {
			body = append(body, node.Statement)
		}
		if _, ok := node.Statement.(*statements.ReturnStatement); ok {
			return body, true
		}
		if custom, ok := node.Statement.(*statements.CustomStatement); ok {
			if _, thrown := custom.SourceThrowOperand(); thrown {
				return body, true
			}
		}
		if len(node.Next) == 0 {
			return nil, false
		}
		if len(node.Next) != 1 {
			return nil, false
		}
		node = node.Next[0]
	}
	return nil, false
}

func dispatchCaseBody(node *core.Node, members map[*core.Node]bool, stateOf map[*core.Node]int, state *values.JavaRef, exitOf map[*core.Node]int, exitState *values.JavaRef, label string) ([]statements.Statement, bool) {
	prefix := []statements.Statement{}
	if _, middle := node.Statement.(*statements.MiddleStatement); !middle {
		prefix = append(prefix, node.Statement)
	}
	transfer := func(target *core.Node) ([]statements.Statement, bool) {
		if members[target] {
			return []statements.Statement{
				statements.NewAssignStatement(state, values.NewJavaLiteral(stateOf[target], state.Type()), false),
				statements.NewSourceTransferStatement("continue", label),
			}, true
		}
		if index, ok := exitOf[target]; ok {
			return []statements.Statement{
				statements.NewAssignStatement(exitState, values.NewJavaLiteral(index, exitState.Type()), false),
				statements.NewSourceTransferStatement("break", label),
			}, true
		}
		return nil, false
	}
	if condition, ok := node.Statement.(*statements.ConditionStatement); ok {
		if condition.Callback != nil || node.TrueNode == nil || node.FalseNode == nil {
			return nil, false
		}
		trueBody, trueOK := transfer(node.TrueNode())
		falseBody, falseOK := transfer(node.FalseNode())
		if !trueOK || !falseOK {
			return nil, false
		}
		return []statements.Statement{statements.NewIfStatement(condition.Condition, trueBody, falseBody)}, true
	}
	if middle, ok := node.Statement.(*statements.MiddleStatement); ok && middle.Flag == statements.MiddleSwitch {
		data, ok := middle.Data.([]any)
		if !ok || len(data) < 2 || node.SwitchCases == nil || node.SwitchDefault == nil {
			return nil, false
		}
		selector, ok := data[1].(values.JavaValue)
		if !ok {
			return nil, false
		}
		items := []*statements.CaseItem{}
		node.SwitchCases.ForEach(func(key int, target *core.Node) bool {
			body, valid := transfer(target)
			if !valid {
				items = nil
				return false
			}
			items = append(items, statements.NewCaseItem(key, body))
			return true
		})
		if items == nil {
			return nil, false
		}
		body, valid := transfer(node.SwitchDefault)
		if !valid {
			return nil, false
		}
		def := statements.NewCaseItem(0, body)
		def.IsDefault = true
		items = append(items, def)
		return []statements.Statement{statements.NewSwitchStatement(selector, items)}, true
	}
	if len(node.Next) != 1 {
		return nil, false
	}
	tail, ok := transfer(node.Next[0])
	if !ok {
		return nil, false
	}
	return append(prefix, tail...), true
}
