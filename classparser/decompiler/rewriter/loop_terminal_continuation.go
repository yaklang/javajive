package rewriter

import (
	"sort"
	"sync"

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

func exclusiveTerminalLoopBranchWithLive(entry, owner *core.Node, dom map[*core.Node][]*core.Node, live func(*core.Node) bool) bool {
	return exclusiveTerminalRegionWithLive(entry, owner, dom, live)
}

func exclusiveTerminalRegion(entry, owner *core.Node, dom map[*core.Node][]*core.Node) bool {
	return exclusiveTerminalRegionWithLive(entry, owner, dom, nil)
}

func domLiveSet(dom map[*core.Node][]*core.Node) map[*core.Node]struct{} {
	live := make(map[*core.Node]struct{}, len(dom))
	for parent, children := range dom {
		live[parent] = struct{}{}
		for _, child := range children {
			live[child] = struct{}{}
		}
	}
	return live
}

func exclusiveTerminalRegionWithLive(entry, owner *core.Node, dom map[*core.Node][]*core.Node, live func(*core.Node) bool) bool {
	if owner != nil && live == nil {
		liveSet := domLiveSet(dom)
		live = func(n *core.Node) bool {
			_, ok := liveSet[n]
			return ok
		}
	}
	incoming := 0
	for _, source := range entry.Source {
		if owner == nil {
			incoming++
			continue
		}
		if live(source) {
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
			if state[source] != 0 {
				continue
			}
			if owner == nil {
				return false
			}
			if live(source) {
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

// A non-terminal region is not necessarily a normal exit: a protected arm or
// an already wrapped container can still carry a continue to this same loop.
// Preserve both explicit CFG edges and the transfers captured by containers.
func loopExitCannotResumeOwner(entry, owner *core.Node) bool {
	if entry == nil || owner == nil {
		return false
	}
	seen := map[*core.Node]bool{}
	queue := []*core.Node{entry}
	for len(queue) > 0 {
		n := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if n == nil || n == owner || len(seen) >= 4096 {
			return false
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		if n.EncodedJumps[owner] {
			return false
		}
		if !isMethodTerminal(n) && !IsEndNode(n) {
			next := loopAnalysisSuccessors(n)
			if len(next) == 0 {
				return false
			}
			queue = append(queue, next...)
		}
	}
	return true
}

// An early return/throw does not flow through a normal loop continuation. A
// strict post-dominator therefore misses a shared break target when an exit
// branch may either return or reach it. Require reachability from every exit
// entry, and prove every bypass path terminates the method. Unknown sinks and
// cycles cannot justify lifting a continuation out of the loop.
type commonExitDistance struct {
	generation uint64
	distance   int
}

type commonExitScratch struct {
	distances  map[*core.Node]commonExitDistance
	common     map[*core.Node]int
	order      map[*core.Node]int
	queue      []*core.Node
	generation uint64
}

var commonExitPool = sync.Pool{New: func() any {
	return &commonExitScratch{
		distances: make(map[*core.Node]commonExitDistance),
		common:    make(map[*core.Node]int),
		order:     make(map[*core.Node]int),
	}
}}

func commonLoopExit(exits []*core.Node) *core.Node {
	if len(exits) == 0 {
		return nil
	}
	scratch := commonExitPool.Get().(*commonExitScratch)
	defer func() {
		clear(scratch.distances)
		clear(scratch.common)
		clear(scratch.order)
		scratch.queue = scratch.queue[:0]
		commonExitPool.Put(scratch)
	}()
	clear(scratch.common)
	clear(scratch.order)
	for i, entry := range exits {
		scratch.generation++
		if scratch.generation == 0 {
			clear(scratch.distances)
			scratch.generation = 1
		}
		generation := scratch.generation
		scratch.distances[entry] = commonExitDistance{generation: generation}
		queue := append(scratch.queue[:0], entry)
		for head := 0; head < len(queue); head++ {
			n := queue[head]
			if isMethodTerminal(n) || IsEndNode(n) {
				continue
			}
			distance := scratch.distances[n].distance
			for _, next := range loopAnalysisSuccessors(n) {
				if known, seen := scratch.distances[next]; seen && known.generation == generation {
					continue
				}
				scratch.distances[next] = commonExitDistance{generation: generation, distance: distance + 1}
				queue = append(queue, next)
			}
		}
		if i == 0 {
			for rank, n := range queue {
				scratch.common[n] = scratch.distances[n].distance
				scratch.order[n] = rank
			}
			scratch.queue = queue[:0]
			continue
		}
		for candidate := range scratch.common {
			if d, reachable := scratch.distances[candidate]; reachable && d.generation == generation {
				scratch.common[candidate] += d.distance
			} else {
				delete(scratch.common, candidate)
			}
		}
		scratch.queue = queue[:0]
	}
	common := scratch.common
	order := scratch.order
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

// Only a rejecting guard may yield the loop boundary to a shared success path.
// A normal header return remains the canonical exit even when a catch has a
// shared rethrow: lifting that rethrow out loses its exception variable scope.
func terminalRegionOnlyThrows(entry *core.Node) bool {
	state := map[*core.Node]uint8{}
	var visit func(*core.Node) bool
	visit = func(n *core.Node) bool {
		if n == nil || len(state) >= 512 {
			return false
		}
		if state[n] != 0 {
			return state[n] == 2
		}
		state[n] = 1
		if isMethodTerminal(n) {
			_, returns := n.Statement.(*statements.ReturnStatement)
			if returns {
				return false
			}
		} else {
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
	return visit(entry)
}
