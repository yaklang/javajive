package core

import (
	"fmt"
	"testing"
)

// reachingScaleGraph builds a linear method that writes every slot once and
// then loads each slot in rounds. It exercises the production slot API rather
// than the sparse solver internals.
func reachingScaleGraph(slots, rounds int) (*SemanticCFG, []struct {
	op   *OpCode
	slot int
}) {
	g := &SemanticCFG{
		order:    map[*OpCode]int{},
		incoming: map[*OpCode][]int{},
		outgoing: map[*OpCode][]int{},
		slots:    map[int]*slotDefinitions{},
	}
	nodes := make([]*OpCode, 0, slots+slots*rounds)
	add := func(opcode int, slot, offset int) *OpCode {
		op := &OpCode{Instr: InstrInfos[opcode], CurrentOffset: uint16(offset), Data: []byte{byte(slot)}}
		g.order[op] = len(nodes)
		nodes = append(nodes, op)
		return op
	}
	offset := 0
	for slot := 0; slot < slots; slot++ {
		add(OP_ISTORE, slot, offset)
		offset++
	}
	loads := make([]struct {
		op   *OpCode
		slot int
	}, 0, slots*rounds)
	for round := 0; round < rounds; round++ {
		for slot := 0; slot < slots; slot++ {
			loads = append(loads, struct {
				op   *OpCode
				slot int
			}{add(OP_ILOAD, slot, offset), slot})
			offset++
		}
	}
	g.Nodes = nodes
	for index := 0; index+1 < len(nodes); index++ {
		edge := len(g.Edges)
		g.Edges = append(g.Edges, SemanticEdge{From: nodes[index], To: nodes[index+1], Kind: EdgeFallthrough})
		g.outgoing[nodes[index]] = append(g.outgoing[nodes[index]], edge)
		g.incoming[nodes[index+1]] = append(g.incoming[nodes[index+1]], edge)
	}
	return g, loads
}

func TestReachingScaleFactsStayOrdered(t *testing.T) {
	g, loads := reachingScaleGraph(8, 4)
	for _, load := range loads {
		defs, _ := g.ReachingDefinitions(load.op, load.slot)
		if len(defs) != 1 {
			t.Fatalf("slot %d load defs = %v", load.slot, defOffsets(defs))
		}
		if got := GetStoreIdx(defs[0]); got != load.slot {
			t.Fatalf("slot %d load resolved store slot %d", load.slot, got)
		}
	}
}

func BenchmarkReachingDefinitionsManySlots(b *testing.B) {
	for _, size := range []int{8, 16, 32} {
		b.Run(fmt.Sprintf("%dx%d", size, size), func(b *testing.B) {
			g, loads := reachingScaleGraph(size, size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				g.slots = map[int]*slotDefinitions{}
				for _, load := range loads {
					if _, _ = g.ReachingDefinitions(load.op, load.slot); g.Err != nil {
						b.Fatal(g.Err)
					}
				}
			}
		})
	}
}

func BenchmarkReachingDefinitionsRepeatQuery(b *testing.B) {
	g, loads := reachingScaleGraph(16, 16)
	for _, load := range loads {
		_, _ = g.ReachingDefinitions(load.op, load.slot)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		load := loads[i%len(loads)]
		if defs, _ := g.ReachingDefinitions(load.op, load.slot); len(defs) != 1 {
			b.Fatalf("defs = %v", defOffsets(defs))
		}
	}
}
