package core

import (
	"testing"
	"unsafe"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestOpCodeStaysCompact(t *testing.T) {
	// Every decoded instruction is an OpCode; a large class allocates hundreds
	// of thousands. Field grouping keeps this at 320 bytes instead of 392.
	if size := unsafe.Sizeof(OpCode{}); size > 320 {
		t.Fatalf("OpCode is %d bytes, want <= 320", size)
	}
}

func performanceStatements(count int) []statements.Statement {
	out := make([]statements.Statement, count)
	for i := range out {
		text := "statement;\n"
		out[i] = statements.NewCustomStatement(func(*class_context.ClassContext) string { return text }, nil)
	}
	return out
}

func TestStatementsStringExactOutput(t *testing.T) {
	items := performanceStatements(3)
	const want = "statement;\nstatement;\nstatement;\n"
	if got := StatementsString(items, &class_context.ClassContext{}); got != want {
		t.Fatalf("StatementsString() = %q, want %q", got, want)
	}
	if got := statements.StatementsString(items, &class_context.ClassContext{}); got != want {
		t.Fatalf("statements.StatementsString() = %q, want %q", got, want)
	}
}

func BenchmarkStatementsStringBuilder(b *testing.B) {
	items := performanceStatements(4096)
	ctx := &class_context.ClassContext{}
	b.ReportAllocs()
	b.SetBytes(int64(len(items) * len("statement;\n")))
	for i := 0; i < b.N; i++ {
		if got := StatementsString(items, ctx); len(got) == 0 {
			b.Fatal("empty output")
		}
	}
}

func TestScanJmpCatchTypesPreserveFirstSeenOrder(t *testing.T) {
	d := NewDecompiler([]byte{OP_NOP, OP_RETURN, OP_POP, OP_RETURN}, nil)
	d.ExceptionTable = []*ExceptionTableEntry{
		{StartPc: 0, EndPc: 2, HandlerPc: 2, CatchType: 7},
		{StartPc: 0, EndPc: 2, HandlerPc: 2, CatchType: 9},
		{StartPc: 0, EndPc: 2, HandlerPc: 2, CatchType: 7},
	}
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	if err := d.ScanJmp(); err != nil {
		t.Fatal(err)
	}
	handler := d.opCodes[d.offsetToOpcodeIndex[2]]
	if got := handler.ExceptionTypeIndexes; len(got) != 2 || got[0] != 7 || got[1] != 9 {
		t.Fatalf("catch types = %v, want [7 9]", got)
	}
}

func BenchmarkScanJmpIndexedExceptionStarts(b *testing.B) {
	const codeSize = 4096
	code := make([]byte, codeSize)
	for i := range code {
		code[i] = OP_NOP
	}
	code[codeSize-1] = OP_RETURN
	table := make([]*ExceptionTableEntry, 0, 1024)
	for i := 0; i < 1024; i++ {
		start := uint16(i * 3)
		table = append(table, &ExceptionTableEntry{StartPc: start, EndPc: start + 1, HandlerPc: codeSize - 1, CatchType: uint16(i%31 + 1)})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := NewDecompiler(code, nil)
		d.ExceptionTable = table
		if err := d.ParseOpcode(); err != nil {
			b.Fatal(err)
		}
		if err := d.ScanJmp(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSemanticCFGAdjacencySnapshotsAreImmutable(t *testing.T) {
	g := t26ExceptionCFG()
	plain := g.successorIndexLists(false)
	withException := g.successorIndexLists(true)
	if len(plain[0]) != 1 || plain[0][0] != 1 {
		t.Fatalf("plain successors = %v", plain)
	}
	if len(withException[0]) != 2 || withException[0][0] != 1 || withException[0][1] != 2 {
		t.Fatalf("exception successors = %v", withException)
	}
	plain[0][0] = 99
	withException[0] = nil
	if got := g.successorIndexLists(false); got[0][0] != 1 {
		t.Fatalf("plain adjacency cache poisoned: %v", got)
	}
	if got := g.successorIndexLists(true); len(got[0]) != 2 {
		t.Fatalf("exception adjacency cache poisoned: %v", got)
	}
}

func BenchmarkSemanticCFGCachedAdjacency(b *testing.B) {
	const nodes = 4096
	edges := make([]t09Edge, 0, nodes*2)
	for i := 0; i+1 < nodes; i++ {
		edges = append(edges, t09Edge{i, i + 1, EdgeFallthrough})
		if i+17 < nodes {
			edges = append(edges, t09Edge{i, i + 17, EdgeTaken})
		}
	}
	g := t09Graph(nodes, edges...)
	domain := GraphAnalysisDomain{Roots: []int{0}}
	_ = g.GetOrCompute(AnalysisDominators, domain)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := g.GetOrCompute(AnalysisDominators, domain); got == nil {
			b.Fatal("nil analysis")
		}
	}
}
