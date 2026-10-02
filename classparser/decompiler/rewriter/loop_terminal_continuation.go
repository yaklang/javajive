package rewriter

import (
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

func isMethodTerminal(n *core.Node) bool {
	if _, ok := n.Statement.(*statements.ReturnStatement); ok {
		return true
	}
	_, custom := n.Statement.(*statements.CustomStatement)
	return custom && renderHead(n.Statement) == "throw"
}

// A return arm may update state or select between several returns first.
// Its entire exclusive region belongs in that arm. Treating only the final
// return as terminal promotes the preceding effects to a loop continuation,
// which can drop nested-loop exits and needlessly move local definitions.
// Keep shared entries, joins from outside the region and unknown sinks
// conservative: those may be a real continuation or a nonlocal control transfer.
func exclusiveTerminalBranch(entry *core.Node) bool {
	return exclusiveTerminalRegion(entry, nil, nil)
}

// A loop exit is terminal only relative to its loop owner. Returning to an
// ancestor header resumes that loop even if the entire method CFG is cyclic
// and has no explicit entry predecessor. Internal nested loops may stay in an
// exclusive arm, but transfers to the current/ancestor loop never may.
func exclusiveTerminalLoopBranch(entry, owner *core.Node, dom map[*core.Node][]*core.Node) bool {
	return exclusiveTerminalRegion(entry, owner, dom)
}
func exclusiveTerminalRegion(entry, owner *core.Node, dom map[*core.Node][]*core.Node) bool {
	live := map[*core.Node]bool{}
	if owner != nil {
		for parent, children := range dom {
			live[parent] = true
			for _, child := range children {
				live[child] = true
			}
		}
	}
	incoming := 0
	for _, source := range entry.Source {
		if owner == nil || live[source] {
			incoming++
		}
	}
	if incoming > 1 {
		return false
	}
	state := map[*core.Node]uint8{}
	var visit func(*core.Node) bool
	visit = func(n *core.Node) bool {
		if n == nil || len(state) > 512 {
			return false
		}
		if owner != nil {
			for target, abrupt := range n.EncodedJumps {
				if abrupt && (target == owner || utils.IsDominate(dom, target, owner)) {
					return false
				}
			}
			if n.IsJmp {
				for _, target := range loopAnalysisSuccessors(n) {
					if target == owner || utils.IsDominate(dom, target, owner) {
						return false
					}
				}
			}
			if n == owner {
				return false
			}
			if _, loop := n.Statement.(*statements.DoWhileStatement); loop && utils.IsDominate(dom, n, owner) {
				return false
			}
		}
		if state[n] != 0 {
			if state[n] == 2 {
				return true
			}
			// A nested natural loop is a closed part of this arm. Unknown
			// cycles and a cycle through the arm entry remain unproved.
			_, nested := n.Statement.(*statements.DoWhileStatement)
			return owner != nil && nested && n != entry
		}
		state[n] = 1
		// A structured try/if can end in a switch break or outer continue.
		// It has no normal fall-through and must retain its lexical owner too.
		terminal := statementIsTerminal(n.Statement)
		if loop, ok := n.Statement.(*statements.DoWhileStatement); owner != nil && ok && len(loop.Body) == 0 {
			// RebuildLoopNode creates a true/empty wrapper before collecting
			// its body. It still has real CFG successors; its empty AST is
			// not evidence of an infinite or method-terminal region.
			terminal = false
		}
		if !terminal {
			next := loopAnalysisSuccessors(n)
			if len(next) == 0 {
				return false
			}
			for _, target := range next {
				if !visit(target) {
					return false
				}
			}
		}
		state[n] = 2
		return true
	}
	if !visit(entry) {
		return false
	}
	for n := range state {
		if n == entry {
			continue
		}
		if len(n.Source) == 0 {
			return false
		}
		for _, source := range n.Source {
			if state[source] == 0 && (owner == nil || live[source]) {
				return false
			}
		}
	}
	return true
}

// Changing a terminal header's role is necessary only when an alternative
// resumes an enclosing loop. Ordinary loops keep their canonical header exit;
// turning every final return into an inline arm needlessly changes variable
// scopes and can confuse later region collection.
func hasEnclosingLoopContinuation(exits []*core.Node, loop *core.Node, dom map[*core.Node][]*core.Node) bool {
	queue := append([]*core.Node(nil), exits...)
	seen := map[*core.Node]bool{loop: true}
	for i := 0; i < len(queue); i++ {
		n := queue[i]
		if seen[n] {
			continue
		}
		seen[n] = true
		if _, header := n.Statement.(*statements.DoWhileStatement); header && utils.IsDominate(dom, n, loop) {
			return true
		}
		if !isMethodTerminal(n) && !IsEndNode(n) {
			queue = append(queue, loopAnalysisSuccessors(n)...)
		}
	}
	return false
}

// An early return/throw does not flow through a normal loop continuation. A
// strict post-dominator therefore misses a shared break target when an exit
// branch may either return or reach it. Require reachability from every exit
// entry, and prove every bypass path terminates the method. Unknown sinks and
// cycles cannot justify lifting a continuation out of the loop.
func commonLoopExit(exits []*core.Node) *core.Node {
	if len(exits) == 0 {
		return nil
	}
	common := map[*core.Node]int{}
	order := map[*core.Node]int{}
	for i, entry := range exits {
		distance := map[*core.Node]int{entry: 0}
		queue := []*core.Node{entry}
		for head := 0; head < len(queue); head++ {
			n := queue[head]
			if isMethodTerminal(n) || IsEndNode(n) {
				continue
			}
			for _, next := range loopAnalysisSuccessors(n) {
				if _, seen := distance[next]; !seen {
					distance[next] = distance[n] + 1
					queue = append(queue, next)
				}
			}
		}
		if i == 0 {
			common = distance
			for rank, n := range queue {
				order[n] = rank
			}
			continue
		}
		for candidate := range common {
			if d, reachable := distance[candidate]; reachable {
				common[candidate] += d
			} else {
				delete(common, candidate)
			}
		}
	}
	var candidates []*core.Node
	for candidate := range common {
		if !IsEndNode(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if common[candidates[i]] != common[candidates[j]] {
			return common[candidates[i]] < common[candidates[j]]
		}
		return order[candidates[i]] < order[candidates[j]]
	})
	for _, candidate := range candidates {
		state := map[*core.Node]uint8{}
		var covered func(*core.Node) bool
		covered = func(n *core.Node) bool {
			if n == candidate || isMethodTerminal(n) {
				return true
			}
			if state[n] != 0 {
				return state[n] == 2 // a grey node is a non-terminating bypass
			}
			state[n] = 1
			next := loopAnalysisSuccessors(n)
			if len(next) == 0 {
				return false
			}
			for _, target := range next {
				if !covered(target) {
					return false
				}
			}
			state[n] = 2
			return true
		}
		valid := true
		for _, entry := range exits {
			if !covered(entry) {
				valid = false
				break
			}
		}
		if valid {
			return candidate
		}
	}
	return nil
}
