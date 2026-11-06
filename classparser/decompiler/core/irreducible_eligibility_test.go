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

func TestIrreducibleBoundaryStackCertificate(t *testing.T) {
	newSingleEdge := func() (*SemanticCFG, *Decompiler) {
		graph := t09Graph(2, t09Edge{0, 1, EdgeTaken})
		for _, node := range graph.Nodes {
			node.Instr = &Instruction{OpCode: OP_NOP}
			node.StackEntry = NewEmptyStackEntry()
		}
		graph.Nodes[0].Instr.OpCode = OP_GOTO
		return graph, &Decompiler{semanticCFG: graph}
	}

	graph, decompiler := newSingleEdge()
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("stack-empty control boundaries were rejected")
	}

	graph.Nodes[0].StackEntry = newStackItem(NewEmptyStackEntry(), values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("a constant carried across a goto boundary was rejected")
	}

	// An effectful word with no producer evidence cannot be re-evaluated at the
	// dispatcher target.
	effectful := values.NewJavaClassMember("X", "value", "I", types.NewJavaPrimer(types.JavaInteger))
	graph.Nodes[0].StackEntry = newStackItem(NewEmptyStackEntry(), effectful)
	if decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("unproved effectful boundary word was admitted")
	}

	// Producer -> boundary directly: the value may be evaluated in the target
	// case because only the control instruction lies in between.
	producer := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 10}
	source := graph.Nodes[0]
	source.CurrentOffset = 13
	producer.Target = []*OpCode{source}
	source.Source = []*OpCode{producer}
	decompiler.stackValueProducers = map[values.JavaValue]*OpCode{effectful: producer}
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("effectful value with an effect-free carrier path was rejected")
	}

	// An intervening field read or write makes delayed evaluation observable.
	barrier := &OpCode{Instr: &Instruction{OpCode: OP_GETFIELD}, CurrentOffset: 11}
	producer.Target = []*OpCode{barrier}
	barrier.Source = []*OpCode{producer}
	barrier.Target = []*OpCode{source}
	source.Source = []*OpCode{barrier}
	if decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("effectful boundary word crossed an observable carrier barrier")
	}

	// Reference words are not covered by primitive stack-lifetime snapshots, so
	// a local store on the carrier path is also a barrier for them.
	refValue := values.NewJavaClassMember("X", "ref", "Ljava/lang/Object;", types.NewJavaClass("java.lang.Object"))
	graph.Nodes[0].StackEntry = newStackItem(NewEmptyStackEntry(), refValue)
	producer.Target = []*OpCode{source}
	source.Source = []*OpCode{producer}
	decompiler.stackValueProducers = map[values.JavaValue]*OpCode{refValue: producer}
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("effectful reference with an immediate producer was rejected")
	}
	store := &OpCode{Instr: &Instruction{OpCode: OP_ISTORE}, CurrentOffset: 11}
	producer.Target = []*OpCode{store}
	store.Source = []*OpCode{producer}
	store.Target = []*OpCode{source}
	source.Source = []*OpCode{store}
	if decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("reference boundary word crossed a local store without a snapshot")
	}
}

func TestIrreducibleBoundaryLocalRefSlotBarrier(t *testing.T) {
	graph := t09Graph(2, t09Edge{0, 1, EdgeTaken})
	for _, node := range graph.Nodes {
		node.Instr = &Instruction{OpCode: OP_GOTO}
		node.StackEntry = NewEmptyStackEntry()
	}
	ref := values.NewJavaRef(nil, values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object")), types.NewJavaClass("java.lang.Object"))
	graph.Nodes[0].StackEntry = newStackItem(NewEmptyStackEntry(), ref)
	decompiler := &Decompiler{
		semanticCFG:         graph,
		stackValueProducers: map[values.JavaValue]*OpCode{},
	}
	producer := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD}, CurrentOffset: 10, Data: []byte{4}}
	source := graph.Nodes[0]
	source.CurrentOffset = 13
	producer.Target = []*OpCode{source}
	source.Source = []*OpCode{producer}
	decompiler.stackValueProducers[values.JavaValue(ref)] = producer
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("local reference with no intervening store was rejected")
	}

	otherStore := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE}, CurrentOffset: 11, Data: []byte{5}}
	producer.Target = []*OpCode{otherStore}
	otherStore.Source = []*OpCode{producer}
	otherStore.Target = []*OpCode{source}
	source.Source = []*OpCode{otherStore}
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("local reference across an unrelated store was rejected")
	}

	sameStore := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE}, CurrentOffset: 11, Data: []byte{4}}
	producer.Target = []*OpCode{sameStore}
	sameStore.Source = []*OpCode{producer}
	sameStore.Target = []*OpCode{source}
	source.Source = []*OpCode{sameStore}
	if decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("local reference across a same-slot store was admitted")
	}
}

func TestIrreducibleBoundaryStackMergeCertificate(t *testing.T) {
	newMerge := func() (*SemanticCFG, *Decompiler) {
		graph := t09Graph(3,
			t09Edge{0, 1, EdgeTaken},
			t09Edge{1, 2, EdgeFallthrough},
			t09Edge{0, 2, EdgeTaken},
		)
		for _, node := range graph.Nodes {
			node.Instr = &Instruction{OpCode: OP_GOTO}
			node.StackEntry = NewEmptyStackEntry()
		}
		graph.Nodes[0].Instr.OpCode = OP_GOTO
		return graph, &Decompiler{semanticCFG: graph}
	}
	literal := func(value int) values.JavaValue {
		return values.NewJavaLiteral(value, types.NewJavaPrimer(types.JavaInteger))
	}

	// Equal per-path constants need no phi.
	graph, decompiler := newMerge()
	graph.Nodes[0].StackEntry = newStackItem(NewEmptyStackEntry(), literal(1))
	graph.Nodes[1].StackEntry = newStackItem(NewEmptyStackEntry(), literal(1))
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("equal incoming constants were rejected")
	}

	// Unequal per-path constants without a materialized phi are a silent
	// miscompile: the target would reuse the first path's value everywhere.
	graph, decompiler = newMerge()
	graph.Nodes[0].StackEntry = newStackItem(NewEmptyStackEntry(), literal(0))
	graph.Nodes[1].StackEntry = newStackItem(NewEmptyStackEntry(), literal(1))
	if decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("unequal incoming constants without a phi were admitted")
	}

	// A proven stack-merge phi carries per-path values safely.
	phi := values.NewJavaRef(nil, nil, types.NewJavaPrimer(types.JavaInteger))
	phi.MarkStackMergePhi()
	graph.Nodes[2].stackConsumed = []values.JavaValue{phi}
	if !decompiler.irreducibleBoundaryStackSupported() {
		t.Fatal("a materialized stack-merge phi was rejected")
	}
}
