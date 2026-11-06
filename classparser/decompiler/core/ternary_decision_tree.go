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

// acyclicPhiHasProtectedJoin reports whether the merge or one of its incoming
// edges sits inside an exception table row. Such a join may already be owned by
// the protected stack-store lowering.
func (d *Decompiler) acyclicPhiHasProtectedJoin(merge *OpCode) bool {
	if d == nil || merge == nil {
		return true
	}
	inside := func(code *OpCode) bool {
		if code == nil {
			return true
		}
		for _, entry := range d.ExceptionTable {
			if code.CurrentOffset >= entry.StartPc && code.CurrentOffset < entry.EndPc {
				return true
			}
		}
		return false
	}
	if inside(merge) {
		return true
	}
	for _, source := range merge.Source {
		if inside(source) {
			return true
		}
	}
	return false
}

// commonSwitchAncestor reports whether every incoming edge of the merge is
// reachable from one switch opcode. Switch expression joins inside a try body
// are safe to materialize because each edge belongs to a case arm; an arbitrary
// protected normal/catch merge is not.
func (d *Decompiler) commonSwitchAncestor(merge *OpCode) bool {
	if merge == nil || len(merge.Source) < 2 || len(merge.Source) > 512 {
		return false
	}
	var common map[*OpCode]bool
	for _, pred := range merge.Source {
		found := map[*OpCode]bool{}
		seen := map[*OpCode]bool{}
		pending := []*OpCode{pred}
		for steps := 0; len(pending) > 0 && steps < 4096; steps++ {
			node := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if node == nil || node.Instr == nil || seen[node] || node == merge {
				continue
			}
			seen[node] = true
			for _, source := range node.Source {
				if source == nil || source.Instr == nil || source == merge {
					continue
				}
				if IsSwitchOpcode(source.Instr.OpCode) {
					found[source] = true
				}
				pending = append(pending, source)
			}
		}
		if common == nil {
			common = found
		} else {
			for candidate := range common {
				if !found[candidate] {
					delete(common, candidate)
				}
			}
		}
		if len(common) == 0 {
			return false
		}
	}
	return len(common) > 0
}

// acyclicPhiHasCatchAncestor reports whether any incoming edge is produced on a
// catch-handler path. Such a merge is owned by the protected stack-store
// lowering; a synthetic edge assignment would be dropped or reordered by the
// try/catch structurer.
func acyclicPhiHasCatchAncestor(merge *OpCode) bool {
	if merge == nil || len(merge.Source) > 512 {
		return true
	}
	for _, pred := range merge.Source {
		seen := map[*OpCode]bool{}
		pending := []*OpCode{pred}
		for steps := 0; len(pending) > 0 && steps < 4096; steps++ {
			node := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if node == nil || node.Instr == nil || seen[node] || node == merge {
				continue
			}
			if node.IsCatch {
				return true
			}
			seen[node] = true
			pending = append(pending, node.Source...)
		}
	}
	return false
}
