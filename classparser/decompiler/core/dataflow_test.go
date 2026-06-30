package core

import "testing"

// op builds a minimal synthetic opcode carrying only the fields computeSlotWebs reads: the
// instruction opcode (to classify store/load + slot via the _N forms) and the backward CFG edges.
func op(opcode int, offset uint16) *OpCode {
	return &OpCode{Instr: &Instruction{OpCode: opcode}, CurrentOffset: offset}
}

// TestSlotWebPartition exercises the reaching-definition web engine (dataflow.go) directly on a
// hand-built CFG that combines the two properties the engine must get right, independent of DFS order:
//
//	n0: istore_1   def X1
//	n1: (branch)        \-> n2 (then) , -> n3 (else)
//	n2: istore_1   def X2  (reassign in the then-arm)
//	n3: (nop)             (else-arm leaves X1 live)
//	n4: iload_1    use    (merge: BOTH X1 and X2 reach -> phi -> ONE variable)
//	n5: istore_1   def Y  (disjoint reuse of slot 1 after the merged value is dead)
//	n6: iload_1    use Y
//
// Expected webs: {n0,n2,n4} (the branch-merged variable) and {n5,n6} (the disjoint reuse), proving
// (1) two definitions converging on one read are unified, and (2) a later disjoint live range that
// merely reuses the JVM slot becomes a distinct variable.
func TestSlotWebPartition(t *testing.T) {
	n0 := op(OP_ISTORE_1, 0)
	n1 := op(OP_IADD, 1) // any non store/load: the branch is encoded purely via Source edges
	n2 := op(OP_ISTORE_1, 2)
	n3 := op(OP_IADD, 3)
	n4 := op(OP_ILOAD_1, 4)
	n5 := op(OP_ISTORE_1, 5)
	n6 := op(OP_ILOAD_1, 6)

	// Backward CFG edges (Source = predecessors).
	n1.Source = []*OpCode{n0}
	n2.Source = []*OpCode{n1}
	n3.Source = []*OpCode{n1}
	n4.Source = []*OpCode{n2, n3}
	n5.Source = []*OpCode{n4}
	n6.Source = []*OpCode{n5}

	d := &Decompiler{opCodes: []*OpCode{n0, n1, n2, n3, n4, n5, n6}}
	webs := d.computeSlotWebs()

	sameWeb := func(a, b *OpCode) bool {
		wa, oka := webs.webOf[a]
		wb, okb := webs.webOf[b]
		return oka && okb && wa == wb
	}

	// Property 1: the two definitions and the merged read are one variable (phi unification).
	if !sameWeb(n0, n2) || !sameWeb(n0, n4) {
		t.Errorf("branch-merge defs not unified: n0=%d n2=%d n4=%d",
			webs.webOf[n0], webs.webOf[n2], webs.webOf[n4])
	}
	// Property 2: the disjoint reuse is its own variable.
	if !sameWeb(n5, n6) {
		t.Errorf("disjoint reuse def/use not unified: n5=%d n6=%d", webs.webOf[n5], webs.webOf[n6])
	}
	if sameWeb(n0, n5) {
		t.Errorf("disjoint slot-1 reuse wrongly merged with the earlier variable: web=%d", webs.webOf[n0])
	}

	// Determinism: recomputing yields the identical partition structure.
	webs2 := d.computeSlotWebs()
	if sameWeb(n0, n5) != (webs2.webOf[n0] == webs2.webOf[n5]) ||
		sameWeb(n0, n4) != (webs2.webOf[n0] == webs2.webOf[n4]) {
		t.Errorf("web partition is not deterministic across recomputation")
	}
}
