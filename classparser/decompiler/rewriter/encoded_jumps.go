package rewriter

import "github.com/yaklang/javajive/classparser/decompiler/core"

func encodedJumpTo(node, target *core.Node) bool {
	if node.EncodedJumps[target] {
		return true
	}
	if node.IsJmp {
		head := renderHead(node.Statement)
		return head == "break" || head == "continue"
	}
	return false
}

// A container can leave via an encoded continue and also complete normally.
// Its CFG retains both edges for enclosing-loop analysis. Record which targets
// have ONLY abrupt entries, so a later if does not mistake them for normal
// fall-through and place a shared continuation in only one branch.
func markEncodedJumps(owner *core.Node, body []*core.Node) {
	inside := make(map[*core.Node]bool, len(body))
	for _, node := range body {
		inside[node] = true
	}
	normal, abrupt := map[*core.Node]bool{}, map[*core.Node]bool{}
	for _, node := range body {
		for _, next := range node.Next {
			if inside[next] {
				continue
			}
			if encodedJumpTo(node, next) {
				abrupt[next] = true
			} else {
				normal[next] = true
			}
		}
	}
	owner.EncodedJumps = nil
	for _, next := range owner.Next {
		if abrupt[next] && !normal[next] {
			if owner.EncodedJumps == nil {
				owner.EncodedJumps = map[*core.Node]bool{}
			}
			owner.EncodedJumps[next] = true
		}
	}
}
