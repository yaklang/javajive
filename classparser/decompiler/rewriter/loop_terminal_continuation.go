package rewriter

import (
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func isMethodTerminal(n *core.Node) bool {
	if _, ok := n.Statement.(*statements.ReturnStatement); ok {
		return true
	}
	_, custom := n.Statement.(*statements.CustomStatement)
	return custom && renderHead(n.Statement) == "throw"
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
