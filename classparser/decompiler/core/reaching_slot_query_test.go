package core

import (
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// legacyReachingSlotStoreRefs is the map-returning form kept as an independent
// oracle for the early-exit query. It must not be used by production code.
func legacyReachingSlotStoreRefs(d *Decompiler, start *OpCode, slot int) map[string]*values.JavaRef {
	out := map[string]*values.JavaRef{}
	if start == nil {
		return out
	}
	visited := map[*OpCode]bool{start: true}
	queue := append([]*OpCode(nil), start.Source...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == nil || visited[cur] {
			continue
		}
		visited[cur] = true
		if isLocalStoreOpcode(cur.Instr.OpCode) && GetStoreIdx(cur) == slot {
			if refs, ok := d.opcodeIdToRef[cur]; ok && len(refs) > 0 {
				if ref, ok2 := refs[len(refs)-1][0].(*values.JavaRef); ok2 && ref != nil {
					out[ref.VarUid] = ref
				}
			}
			continue
		}
		queue = append(queue, cur.Source...)
	}
	return out
}

// The early-exit query must answer exactly like the map form followed by the
// repair predicate. Random graphs include branches, merges, duplicate refs,
// missing store refs, and reused scratch generations across queries.
func TestReachingSlotStoreRefUniqueMatchesMapQuery(t *testing.T) {
	rng := rand.New(rand.NewSource(0x51a7))
	refs := []*values.JavaRef{{VarUid: "a"}, {VarUid: "b"}, {VarUid: "c"}}
	for trial := 0; trial < 1500; trial++ {
		n := 6 + rng.Intn(16)
		nodes := make([]*OpCode, n)
		for i := range nodes {
			nodes[i] = &OpCode{Instr: &Instruction{OpCode: OP_NOP}}
		}
		for i := 0; i < n; i++ {
			if rng.Intn(3) == 0 {
				nodes[i].Instr = &Instruction{OpCode: OP_ISTORE}
				nodes[i].Data = []byte{byte(rng.Intn(3))}
			}
		}
		for i := 1; i < n; i++ {
			for edge := 0; edge < 1+rng.Intn(2); edge++ {
				nodes[i].Source = append(nodes[i].Source, nodes[rng.Intn(i)])
			}
		}
		d := &Decompiler{opcodeIdToRef: map[*OpCode][][2]any{}}
		for _, node := range nodes {
			if isLocalStoreOpcode(node.Instr.OpCode) && rng.Intn(2) == 0 {
				ref := refs[rng.Intn(len(refs))]
				d.opcodeIdToRef[node] = [][2]any{{ref, nil}}
			}
		}
		start := nodes[n-1]
		slot := rng.Intn(3)
		for _, current := range append(append([]*values.JavaRef(nil), refs...), nil) {
			legacy := legacyReachingSlotStoreRefs(d, start, slot)
			var want *values.JavaRef
			if len(legacy) == 1 {
				for _, ref := range legacy {
					if current == nil || ref.VarUid != current.VarUid {
						want = ref
					}
				}
			}
			got := d.reachingSlotStoreRefUnique(start, slot, current)
			if (got == nil) != (want == nil) || (got != nil && got.VarUid != want.VarUid) {
				t.Fatalf("trial %d slot %d current %v: got %v want %v (map=%v)", trial, slot, current, got, want, legacy)
			}
		}
	}
}
