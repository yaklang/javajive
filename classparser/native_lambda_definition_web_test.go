package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Abstract access graphs test lifetime partition boundaries. They deliberately
// omit operand stacks and do not claim JVM verifier validity.
func TestNativeLambdaDefinitionWebRequiresDisjointOriginalReadFrontiers(t *testing.T) {
	for _, variant := range []string{"disjoint lifetime", "old counter loop", "joined lifetimes", "captured increment", "uninitialized path", "wide lower overlap", "wide upper overlap", "wrong captured slot", "missing definition", "duplicate definition", "no definitions", "foreign LOAD", "nil instruction", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			ops := make([]*core.OpCode, 9)
			for i := range ops {
				ops[i] = &core.OpCode{CurrentOffset: uint16(i), Instr: &core.Instruction{OpCode: core.OP_NOP}}
			}
			for _, i := range []int{1, 5, 6} {
				ops[i].Instr.OpCode = core.OP_ISTORE_2
			}
			for _, i := range []int{2, 7} {
				ops[i].Instr.OpCode = core.OP_ILOAD_2
			}
			ops[3].Instr.OpCode = core.OP_IINC
			ops[3].Data = []byte{2, 1}
			flow := &nativeEnumParameterFlow{entry: ops[0], byPC: map[int]*core.OpCode{}, edges: map[*core.OpCode][]core.SemanticEdge{}}
			for i, op := range ops {
				flow.byPC[i] = op
			}
			edge := func(from, to int) {
				flow.edges[ops[from]] = append(flow.edges[ops[from]], core.SemanticEdge{From: ops[from], To: ops[to], Kind: core.EdgeTaken})
			}
			for _, pair := range [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {4, 6}, {5, 7}, {6, 7}, {7, 8}} {
				edge(pair[0], pair[1])
			}
			read := &nativeEnumLocalRead{pc: 7, opcode: core.OP_ILOAD_2, slot: 2, descriptor: "I", storePC: -1, storePCs: []int{5, 6}}
			var work *workbudget.Budget
			want := variant == "disjoint lifetime" || variant == "old counter loop"
			switch variant {
			case "old counter loop":
				edge(3, 2)
			case "joined lifetimes":
				edge(5, 2)
			case "captured increment":
				ops[8].Instr.OpCode = core.OP_IINC
				ops[8].Data = []byte{2, 1}
				flow.edges[ops[5]] = nil
				edge(5, 8)
				edge(8, 7)
			case "uninitialized path":
				edge(0, 7)
			case "wide lower overlap":
				ops[1].Instr.OpCode = core.OP_LSTORE_1
			case "wide upper overlap":
				ops[1].Instr.OpCode = core.OP_DSTORE_2
			case "wrong captured slot":
				ops[6].Instr.OpCode = core.OP_ISTORE_3
			case "missing definition":
				read.storePCs = []int{5, 11}
			case "duplicate definition":
				read.storePCs = []int{5, 5}
			case "no definitions":
				read.storePCs = nil
			case "foreign LOAD":
				delete(flow.byPC, 2)
			case "nil instruction":
				ops[1].Instr = nil
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
				t.Fatalf("definition web=%v want=%v", got, want)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("bounded refusal was not sticky")
			}
		})
	}
}
