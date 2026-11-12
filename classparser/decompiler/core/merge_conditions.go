package core

import "slices"

// mergeConditionIndex memoizes the nearest conditional ancestors of a node.
// Stack-simulation value merges used to walk backwards from every predecessor
// separately, rediscovering the same conditions for every merge. A condition is
// terminal for this relation: conditions behind it are owned by a nested value
// tree, exactly as the old per-predecessor WalkGraph implemented.
type mergeConditionIndex struct {
	cache  map[*OpCode][]*OpCode
	active map[*OpCode]bool
}

func newMergeConditionIndex() *mergeConditionIndex {
	// Maps are allocated on first use: most methods have no value merge, and
	// this index is created for every stack simulation.
	return &mergeConditionIndex{}
}

func (m *mergeConditionIndex) ensureMaps() {
	if m.cache == nil {
		m.cache = make(map[*OpCode][]*OpCode, 64)
		m.active = make(map[*OpCode]bool, 16)
	}
}

func isConditionOpcode(op *OpCode) bool {
	if op == nil || op.Instr == nil {
		return false
	}
	switch op.Instr.OpCode {
	case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE,
		OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE,
		OP_IFNONNULL, OP_IFNULL:
		return true
	}
	return false
}

func (m *mergeConditionIndex) nearest(node *OpCode) []*OpCode {
	if node == nil {
		return nil
	}
	m.ensureMaps()
	if cached, ok := m.cache[node]; ok {
		return cached
	}
	if isConditionOpcode(node) {
		out := []*OpCode{node}
		m.cache[node] = out
		return out
	}
	if m.active[node] {
		// A cycle through non-condition nodes contributes no new condition on
		// this path; another path discovers any condition reachable from it.
		return nil
	}
	m.active[node] = true
	var out []*OpCode
	// WalkGraph pushes all successors and pops them LIFO, so the last source is
	// visited first. Iterate the children in reverse to preserve its exact
	// condition order (downstream ternary wiring is order-sensitive).
	for index := len(node.Source) - 1; index >= 0; index-- {
		for _, condition := range m.nearest(node.Source[index]) {
			if !slices.Contains(out, condition) {
				out = append(out, condition)
			}
		}
	}
	delete(m.active, node)
	m.cache[node] = out
	return out
}

// collect returns the nearest conditions of the given predecessor list in
// source order, preserving the old WalkGraph traversal order.
func (m *mergeConditionIndex) collect(sources []*OpCode) []*OpCode {
	var out []*OpCode
	for _, source := range sources {
		for _, condition := range m.nearest(source) {
			if !slices.Contains(out, condition) {
				out = append(out, condition)
			}
		}
	}
	return out
}
