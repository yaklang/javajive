package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Abstract word-access graphs, not verifier-valid JVM programs. A different
// width may reuse dead words, but no LOAD may borrow a partial or mixed writer.
func TestNativeLambdaWordLifetimesRequireWholeTypedReadFrontiers(t *testing.T) {
	for _, variant := range []string{"lower-word reuse", "upper-word reuse", "wide shifted reuse", "capture reached from handler", "catch read before definition", "partial wide read", "wrong writer category", "wrong captured width", "wrong captured category", "wrong frontier", "missing read", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			ops := make([]*core.OpCode, 10)
			flow := &nativeEnumParameterFlow{byPC: map[int]*core.OpCode{}, edges: map[*core.OpCode][]core.SemanticEdge{}}
			for i := range ops {
				ops[i] = &core.OpCode{CurrentOffset: uint16(i), Instr: &core.Instruction{OpCode: core.OP_NOP}}
				flow.byPC[i] = ops[i]
			}
			flow.entry = ops[0]
			ops[1].Instr.OpCode, ops[3].Instr.OpCode = core.OP_LSTORE_2, core.OP_LSTORE_2
			ops[5].Instr.OpCode = core.OP_LLOAD_2
			ops[7].Instr.OpCode, ops[8].Instr.OpCode = core.OP_ASTORE_2, core.OP_ALOAD_2
			edge := func(from, to int, kind core.EdgeKind) {
				flow.edges[ops[from]] = append(flow.edges[ops[from]], core.SemanticEdge{From: ops[from], To: ops[to], Kind: kind})
			}
			for _, pair := range [][2]int{{0, 1}, {0, 3}, {1, 2}, {2, 5}, {3, 4}, {4, 5}, {5, 6}, {6, 9}, {7, 8}, {8, 9}} {
				edge(pair[0], pair[1], core.EdgeFallthrough)
			}
			edge(6, 7, core.EdgeException)
			read := &nativeEnumLocalRead{pc: 5, opcode: core.OP_LLOAD_2, slot: 2, descriptor: "J", storePC: -1, storePCs: []int{1, 3}}
			var work *workbudget.Budget
			want := variant == "lower-word reuse" || variant == "upper-word reuse" || variant == "wide shifted reuse"
			switch variant {
			case "upper-word reuse":
				ops[7].Instr.OpCode, ops[8].Instr.OpCode = core.OP_ASTORE_3, core.OP_ALOAD_3
			case "wide shifted reuse":
				ops[7].Instr.OpCode, ops[8].Instr.OpCode = core.OP_DSTORE_1, core.OP_DLOAD_1
			case "capture reached from handler":
				edge(7, 5, core.EdgeFallthrough)
			case "catch read before definition":
				flow.edges[ops[6]] = nil
				edge(6, 8, core.EdgeException)
			case "partial wide read":
				ops[8].Instr.OpCode = core.OP_LLOAD_2
			case "wrong writer category":
				ops[8].Instr.OpCode = core.OP_ILOAD_2
			case "wrong captured width":
				ops[3].Instr.OpCode = core.OP_ISTORE_2
			case "wrong captured category":
				ops[3].Instr.OpCode = core.OP_DSTORE_2
			case "wrong frontier":
				read.storePCs = []int{1, 7}
			case "missing read":
				delete(flow.byPC, 8)
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeLambdaLocalDefinitionWebClosed(flow, ops, read, work); got != want {
				t.Fatalf("whole typed frontier=%v want=%v", got, want)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("bounded refusal must be sticky")
			}
		})
	}
}
