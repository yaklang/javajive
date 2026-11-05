package core

// ifNodesFormSingleDecisionTree reports whether one of the candidate
// conditions can reach every other candidate before the merge. Nested
// ternary / short-circuit chains have such a root. Independent switch-arm
// conditions do not: the switch dispatcher separates them, so no case arm can
// reach a sibling arm's condition.
func ifNodesFormSingleDecisionTree(ifNodes []*OpCode, merge *OpCode) bool {
	if len(ifNodes) <= 1 {
		return true
	}
	if merge == nil || len(ifNodes) > 128 {
		return false
	}
	wanted := make(map[*OpCode]bool, len(ifNodes))
	for _, node := range ifNodes {
		if node == nil {
			return false
		}
		wanted[node] = true
	}
	for _, root := range ifNodes {
		seen := map[*OpCode]bool{}
		pending := []*OpCode{root}
		found := 0
		for len(pending) > 0 && len(seen) < 4096 {
			node := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if node == nil || node.Instr == nil || seen[node] || node == merge || node.CurrentOffset >= merge.CurrentOffset {
				continue
			}
			seen[node] = true
			if wanted[node] {
				found++
				if found == len(wanted) {
					return true
				}
			}
			pending = append(pending, node.Target...)
		}
	}
	return false
}
