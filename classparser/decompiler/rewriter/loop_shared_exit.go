package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// Factor only a shared continuation reached through complete, loop-owned
// exit prefixes. Effects in each prefix remain in their respective arms.
// An ancestor continuation, foreign entry, protected boundary or cyclic
// bypass cannot establish this region and retains the ordinary header rule.
func ownedSharedLoopExit(exits []*core.Node, owner *core.Node, dom map[*core.Node][]*core.Node) *core.Node {
	if owner == nil || len(exits) < 2 {
		return nil
	}
	join := commonLoopExit(exits)
	if join == nil || join == owner || join.IsCatchStart || join.HasProtectedRange || IsEndNode(join) || len(join.EncodedJumps) != 0 {
		return nil
	}
	// A dominating join resumes an enclosing region through a back edge.
	// Its identity does not depend on whether that region has already been
	// rewritten into a loop statement. It cannot become an ordinary break.
	if utils.IsDominate(dom, join, owner) {
		return nil
	}
	state := map[*core.Node]uint8{}
	var closed func(*core.Node) bool
	closed = func(n *core.Node) bool {
		if n == join {
			return true
		}
		if n == nil || n == owner || n.IsCatchStart || n.HasProtectedRange || len(n.EncodedJumps) != 0 || !utils.IsDominate(dom, owner, n) || len(state) >= 4096 {
			return false
		}
		if state[n] != 0 {
			return state[n] == 2
		}
		state[n] = 1
		if originalLoopThrowLeaf(n) {
			// The original abrupt arm still throws; it is never changed to
			// a break that could reach the common continuation.
			state[n] = 2
			return true
		}
		if isMethodTerminal(n) || IsEndNode(n) {
			return false
		}
		next := loopAnalysisSuccessors(n)
		if len(next) == 0 {
			return false
		}
		for _, successor := range next {
			if !closed(successor) {
				return false
			}
		}
		state[n] = 2
		return true
	}
	for _, exit := range exits {
		if !closed(exit) {
			return nil
		}
	}
	return join
}
