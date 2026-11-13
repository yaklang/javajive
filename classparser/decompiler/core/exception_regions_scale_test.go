package core

import (
	"fmt"
	"testing"
)

func exceptionRegionScaleCFG(t *testing.T, handlers int) *SemanticCFG {
	t.Helper()
	code := make([]byte, 0, handlers*4+1)
	table := make([]*ExceptionTableEntry, 0, handlers)
	for i := 0; i < handlers; i++ {
		start := len(code) + 2
		code = append(code, OP_NOP, OP_NOP, OP_IDIV, OP_ASTORE_1)
		table = append(table, &ExceptionTableEntry{
			StartPc: uint16(start), EndPc: uint16(start + 1),
			HandlerPc: uint16(start + 1), CatchType: uint16(i + 1),
		})
	}
	code = append(code, OP_RETURN)
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	d.ExceptionTable = table
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatalf("buildSemanticCFG: %v", err)
	}
	return g
}

func TestExceptionRegionScaleAcceptsDisjointHandlers(t *testing.T) {
	g := exceptionRegionScaleCFG(t, 256)
	if err := g.ValidateExceptionRegions(); err != nil {
		t.Fatalf("disjoint handler ranges rejected: %v", err)
	}
}

func exceptionCoverageGraph(coverages [][]uint16) *SemanticCFG {
	var nodes []*OpCode
	for pc := uint16(0); pc < 64; pc++ {
		nodes = append(nodes, &OpCode{Instr: InstrInfos[OP_IDIV], CurrentOffset: pc, Id: int(pc)})
	}
	handlers := []*OpCode{
		{Instr: InstrInfos[OP_ASTORE_1], CurrentOffset: 100, Id: 100},
		{Instr: InstrInfos[OP_ASTORE_1], CurrentOffset: 101, Id: 101},
	}
	g := &SemanticCFG{Nodes: nodes, order: map[*OpCode]int{}}
	for index, node := range nodes {
		g.order[node] = index
	}
	for _, handler := range handlers {
		g.order[handler] = len(g.order)
		g.Nodes = append(g.Nodes, handler)
	}
	for handlerOrder, pcs := range coverages {
		for _, pc := range pcs {
			g.Edges = append(g.Edges, SemanticEdge{
				From: nodes[pc], To: handlers[handlerOrder],
				Kind: EdgeException, HandlerOrder: handlerOrder,
			})
		}
	}
	return g
}

func TestExceptionRegionMultiSpanCrossing(t *testing.T) {
	// Coverage A={1,3} and B={2,3}: they intersect, A has 1 outside B, and B
	// has 2 outside A, so the unions cross even though the individual spans
	// [3,3] and [2,3] are nested.
	g := exceptionCoverageGraph([][]uint16{{1, 3}, {2, 3}})
	if err := g.ValidateExceptionRegions(); err == nil {
		t.Fatal("multi-span union crossing was accepted")
	}

	// A={1,2,3}, B={2} is a strict subset: no crossing.
	g = exceptionCoverageGraph([][]uint16{{1, 2, 3}, {2}})
	if err := g.ValidateExceptionRegions(); err != nil {
		t.Fatalf("nested multi-span coverage rejected: %v", err)
	}

	// Disjoint coverage with an interleaved second span is accepted.
	g = exceptionCoverageGraph([][]uint16{{0, 4}, {1, 5}})
	if err := g.ValidateExceptionRegions(); err != nil {
		t.Fatalf("disjoint interleaved coverage rejected: %v", err)
	}
}

func BenchmarkValidateExceptionRegionsManyHandlers(b *testing.B) {
	for _, handlers := range []int{64, 256, 1024} {
		b.Run(fmt.Sprintf("%d", handlers), func(b *testing.B) {
			code := make([]byte, 0, handlers*4+1)
			table := make([]*ExceptionTableEntry, 0, handlers)
			for i := 0; i < handlers; i++ {
				start := len(code) + 2
				code = append(code, OP_NOP, OP_NOP, OP_IDIV, OP_ASTORE_1)
				table = append(table, &ExceptionTableEntry{
					StartPc: uint16(start), EndPc: uint16(start + 1),
					HandlerPc: uint16(start + 1), CatchType: uint16(i + 1),
				})
			}
			code = append(code, OP_RETURN)
			d := NewDecompiler(code, nil)
			if err := d.ParseOpcode(); err != nil {
				b.Fatal(err)
			}
			d.ExceptionTable = table
			g, err := d.buildSemanticCFG()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := g.ValidateExceptionRegions(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
