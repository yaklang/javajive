package rewriter

import (
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
)

type forwardRegion struct {
	exit  *core.Node
	label string
}

type forwardEdge struct{ from, to *core.Node }

// An inner decision can reach a shared computation or bypass it and leave
// an enclosing region. Dominance alone cannot turn both outcomes into normal
// fall-through. Seal the bypass as a region break before collecting the arm;
// later, enclose the proved region in a single-execution source block.
func materializeForwardRegionExits(manager *RewriteManager, condition *core.Node) {
	if manager == nil || condition == nil {
		return
	}
	var frontier []forwardEdge
	seen := map[*core.Node]bool{}
	queue := []*core.Node{condition}
	remaining := 4096
	for len(queue) > 0 {
		n := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if n == nil || len(seen) >= 1024 {
			return
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		for _, next := range n.Next {
			remaining--
			if remaining < 0 {
				return
			}
			if encodedJumpTo(n, next) {
				continue
			}
			if next == condition {
				return
			}
			if utils2.IsDominate(manager.DominatorMap, condition, next) {
				queue = append(queue, next)
			} else {
				frontier = append(frontier, forwardEdge{n, next})
			}
		}
	}
	if len(frontier) < 2 {
		return
	}
	// The continuation may also have predecessors outside this decision
	// (for example an enclosing guard's false arm). Its immediate dominator
	// is then too broad. Find the smallest ancestor containing this decision
	// and every non-exit frontier, and prove that region independently.
	parent := map[*core.Node]*core.Node{}
	work := 65536
	for ancestor, children := range manager.DominatorMap {
		for _, child := range children {
			work--
			if work < 0 || child == nil || parent[child] != nil {
				return
			}
			parent[child] = ancestor
		}
	}
	tried := map[*core.Node]bool{}
	for _, edge := range frontier {
		exit := edge.to
		if tried[exit] {
			continue
		}
		tried[exit] = true
		var owner *core.Node
		for candidate, depth := parent[condition], 0; candidate != nil && depth < 1024; candidate, depth = parent[candidate], depth+1 {
			if _, raw := candidate.Statement.(*statements.ConditionStatement); !raw {
				continue
			}
			other, contains := false, true
			for _, e := range frontier {
				work--
				if work < 0 {
					return
				}
				if e.to != exit {
					other = true
					contains = contains && utils2.IsDominate(manager.DominatorMap, candidate, e.to)
				}
			}
			if other && contains {
				owner = candidate
				break
			}
		}
		if owner == nil || !closedForwardRegion(manager, owner, exit) {
			continue
		}

		region := manager.forwardRegions[owner]
		if region != nil && region.exit != exit {
			continue
		}
		if region == nil {
			region = &forwardRegion{exit: exit, label: manager.NewLoopLabel()}
			if manager.forwardRegions == nil {
				manager.forwardRegions = map[*core.Node]*forwardRegion{}
			}
			manager.forwardRegions[owner] = region
		}
		for _, e := range frontier {
			if e.to != exit || !slices.Contains(e.from.Next, exit) || encodedJumpTo(e.from, exit) {
				continue
			}
			jump := manager.NewNode(statements.NewSourceTransferStatement("break", region.label))
			jump.AddNext(exit)
			jump.EncodedJumps = map[*core.Node]bool{exit: true}
			replaceNextInPlace(e.from, exit, jump)
		}
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
		return
	}
}

// Prove single entry and a closed acyclic normal-flow region. Structured
// inner loops are atomic; raw cycles, hidden/exception edges and external
// transfers cannot be swallowed by this new lexical scope. The proof makes
// no assumptions about local names, values, branch polarity or arm effects.
func closedForwardRegion(manager *RewriteManager, owner, exit *core.Node) bool {
	if manager == nil || !manager.originalHandlersKnown || owner == nil || exit == nil || !owner.HasOriginPC || !exit.HasOriginPC || exit.IsCatchStart || IsEndNode(exit) {
		return false
	}
	remaining := 65536
	sameDomain := func(n *core.Node) bool {
		if n == nil || !n.HasOriginPC || n.OriginPC < 0 || n.OriginPC > 65535 || owner.OriginPC < 0 || owner.OriginPC > 65535 {
			return false
		}
		remaining -= len(manager.originalHandlers) + 1
		if remaining < 0 {
			return false
		}
		for _, row := range manager.originalHandlers {
			contains := func(pc int) bool { return pc >= int(row.StartPc) && pc < int(row.EndPc) }
			if contains(n.OriginPC) != contains(owner.OriginPC) {
				return false
			}
		}
		return true
	}
	if !sameDomain(exit) {
		return false
	}
	state := map[*core.Node]uint8{}
	statementBudget := 4096
	var visit func(*core.Node) bool
	visit = func(n *core.Node) bool {
		if n == exit {
			return true
		}
		if n == nil || len(state) >= 1024 || n.HideNext != nil || n.IsCatchStart || n.IsTryCatch || n.HasProtectedRange || (n != owner && !utils2.IsDominate(manager.DominatorMap, owner, n)) {
			return false
		}
		if state[n] != 0 {
			return state[n] == 2
		}
		if n.HasOriginPC && !sameDomain(n) {
			return false
		}
		switch n.Statement.(type) {
		case *statements.ConditionStatement, *statements.AssignStatement, *statements.ExpressionStatement, *values.JavaExpression:
			if !sameDomain(n) {
				return false
			}
		case *statements.IfStatement, *statements.DoWhileStatement, *statements.WhileStatement:
			if !forwardBlockKeepsTransferBindings(n.Statement, 0, 0, &statementBudget) {
				return false
			}
		case *statements.CustomStatement:
			r := manager.forwardRegions[owner]
			kind, label, ok := n.Statement.(*statements.CustomStatement).SourceTransferProtocol()
			if r == nil || r.exit != exit || !ok || kind != "break" || label != r.label || len(n.Next) != 1 || n.Next[0] != exit || !n.EncodedJumps[exit] {
				return false
			}
		default:
			return false
		}
		if len(n.Next) == 0 {
			return false
		}
		state[n] = 1
		for _, next := range n.Next {
			remaining--
			if remaining < 0 {
				return false
			}
			if encodedJumpTo(n, next) && (next != exit || manager.forwardRegions[owner] == nil) {
				return false
			}
			if !visit(next) {
				return false
			}
		}
		state[n] = 2
		return true
	}
	return visit(owner)
}

// Adding a one-shot loop must not capture an existing unqualified transfer.
// Transfers inside an already structured loop keep that nearer owner. Explicit
// labels keep their exact target. Opaque leaves and protected/monitor scopes
// provide no such certificate and are deliberately left to their own rewrites.
func forwardBlockKeepsTransferBindings(st statements.Statement, loops, depth int, remaining *int) bool {
	*remaining--
	if st == nil || depth > 64 || *remaining < 0 {
		return false
	}
	block := func(body []statements.Statement, loopDepth int) bool {
		for _, child := range body {
			if !forwardBlockKeepsTransferBindings(child, loopDepth, depth+1, remaining) {
				return false
			}
		}
		return true
	}
	switch x := st.(type) {
	case *statements.AssignStatement:
		return x != nil
	case *statements.ExpressionStatement:
		return x != nil
	case *statements.ReturnStatement:
		return x != nil
	case *values.JavaExpression:
		return x != nil
	case *statements.IfStatement:
		return x != nil && x.Condition != nil && block(x.IfBody, loops) && block(x.ElseBody, loops)
	case *statements.DoWhileStatement:
		return x != nil && x.ConditionValue != nil && block(x.Body, loops+1)
	case *statements.WhileStatement:
		return x != nil && x.ConditionValue != nil && block(x.Body, loops+1)
	case *statements.CustomStatement:
		if _, label, ok := x.SourceTransferProtocol(); ok {
			return label != "" || loops > 0
		}
		if _, ok := x.SourceThrowOperand(); ok {
			return true
		}
		_, ok := x.SourceAssignmentForward()
		return ok
	default:
		return false
	}
}

func finishForwardRegion(manager *RewriteManager, original, start *core.Node) error {
	region := manager.forwardRegions[original]
	if region == nil {
		return nil
	}
	var body []*core.Node
	seen := map[*core.Node]bool{}
	for n := start; n != region.exit; {
		if n == nil || seen[n] || len(seen) >= 1024 {
			return fmt.Errorf("forward region did not close")
		}
		if _, raw := n.Statement.(*statements.ConditionStatement); raw {
			return fmt.Errorf("forward region contains an unstructured decision")
		}
		seen[n] = true
		body = append(body, n)
		var normal []*core.Node
		for _, next := range n.Next {
			if encodedJumpTo(n, next) {
				if next != region.exit {
					return fmt.Errorf("forward region has a foreign transfer")
				}
			} else {
				normal = append(normal, next)
			}
		}
		if len(normal) != 1 {
			return fmt.Errorf("forward region has %d normal continuations", len(normal))
		}
		n = normal[0]
	}
	block := statements.NewDoWhileStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), core.NodesToStatements(body))
	block.Label = region.label
	wrapper := manager.NewNode(block)
	manager.replaceStructuredNode(start, wrapper)
	if manager.RootNode == start {
		manager.RootNode = wrapper
	}
	wrapper.RemoveAllNext()
	for _, n := range body {
		n.RemoveAllNext()
	}
	wrapper.AddNext(region.exit)
	retargetProtectedIfBoundary(manager.TryNodes, start, wrapper)
	delete(manager.forwardRegions, original)
	return nil
}

// Pending region exits name graph identities, so every structured replacement
// must retarget those identities together with the incoming CFG edges. The
// source break label still names the same lexical region and is unchanged.
func (manager *RewriteManager) replaceStructuredNode(old, replacement *core.Node) {
	old.Replace(replacement)
	for _, region := range manager.forwardRegions {
		if region.exit == old {
			region.exit = replacement
		}
	}
}
