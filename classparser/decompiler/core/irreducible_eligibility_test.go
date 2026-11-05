package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestIrreducibleDeferralRejectsUnsafeDomains(t *testing.T) {
	newGraph := func() *SemanticCFG {
		graph := t09Graph(3,
			t09Edge{0, 1, EdgeTaken},
			t09Edge{0, 2, EdgeFallthrough},
			t09Edge{1, 2, EdgeTaken},
			t09Edge{2, 1, EdgeTaken},
		)
		for _, node := range graph.Nodes {
			node.Instr = &Instruction{OpCode: OP_NOP}
		}
		return graph
	}

	if graph := newGraph(); !graph.canDeferIrreducible() {
		t.Fatal("ordinary normal-flow region was not eligible for deferred dispatch")
	}
	for _, opcode := range []int{OP_NEW, OP_MONITORENTER, OP_MONITOREXIT} {
		graph := newGraph()
		graph.Nodes[1].Instr.OpCode = opcode
		if graph.canDeferIrreducible() {
			t.Fatalf("opcode 0x%x was admitted to normal-flow dispatch", opcode)
		}
	}
	graph := newGraph()
	graph.Edges = append(graph.Edges, SemanticEdge{From: graph.Nodes[1], To: graph.Nodes[2], Kind: EdgeException})
	if graph.canDeferIrreducible() {
		t.Fatal("exception edge was admitted to normal-flow dispatch")
	}
}

func TestIrreducibleDispatchRequiresEmptyControlBoundaries(t *testing.T) {
	graph := t09Graph(3,
		t09Edge{0, 1, EdgeTaken},
		t09Edge{0, 2, EdgeFallthrough},
		t09Edge{1, 2, EdgeTaken},
		t09Edge{2, 1, EdgeTaken},
	)
	for _, node := range graph.Nodes {
		node.Instr = &Instruction{OpCode: OP_NOP}
		node.StackEntry = NewEmptyStackEntry()
	}
	graph.Nodes[0].Instr.OpCode = OP_IFEQ
	graph.Nodes[1].Instr.OpCode = OP_GOTO
	graph.Nodes[2].Instr.OpCode = OP_GOTO
	decompiler := &Decompiler{semanticCFG: graph}
	if !decompiler.irreducibleEdgesHaveEmptyStack() {
		t.Fatal("stack-empty control boundaries were rejected")
	}

	graph.Nodes[1].StackEntry = newStackItem(NewEmptyStackEntry(), values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
	if decompiler.irreducibleEdgesHaveEmptyStack() {
		t.Fatal("non-empty goto boundary was admitted to source dispatch")
	}
}
