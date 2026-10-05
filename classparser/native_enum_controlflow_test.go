package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumGeneratedRegionUsesControlFlowNotByteOffsets(t *testing.T) {
	for _, row := range []struct {
		name  string
		entry uint16
		edges [][2]uint16
		want  bool
	}{
		{"straight", 0, [][2]uint16{{0, 1}, {1, 2}, {2, 3}}, true},
		{"physically later prelude", 0, [][2]uint16{{0, 8}, {8, 9}, {9, 1}, {1, 2}, {2, 3}}, true},
		{"prefix alternatives", 0, [][2]uint16{{0, 8}, {0, 9}, {8, 1}, {9, 1}, {1, 2}, {2, 3}}, true},
		{"normal return bypass", 0, [][2]uint16{{0, 1}, {0, 3}, {1, 2}, {2, 3}}, false},
		{"unreachable packet", 0, [][2]uint16{{0, 3}, {1, 2}, {2, 3}}, false},
		{"unreachable consumer", 0, [][2]uint16{{0, 1}, {1, 1}, {2, 3}}, false},
		{"repeat packet", 0, [][2]uint16{{0, 1}, {1, 2}, {2, 8}, {8, 1}, {2, 3}}, false},
		{"repeat consumer", 0, [][2]uint16{{0, 1}, {1, 2}, {2, 2}, {2, 3}}, false},
		{"tail loop preserves once", 0, [][2]uint16{{0, 1}, {1, 2}, {2, 8}, {8, 8}, {8, 3}}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			ir := &methodir.MethodIR{EntryPC: row.entry, Instrs: []methodir.Instr{{PC: 3, Opcode: core.OP_RETURN}}}
			for _, edge := range row.edges {
				ir.Edges = append(ir.Edges, methodir.Edge{From: methodir.InstrID(edge[0]), To: methodir.InstrID(edge[1])})
			}
			if got := nativeEnumRegionExecutedOnce(ir, 1, 2, nil); got != row.want {
				t.Fatalf("once=%v want=%v", got, row.want)
			}
		})
	}
}

// Floyd-Warshall is an independent oracle for all 2^12 directed four-node
// graphs. Removing the consumer must separate entry from normal return; its
// successors must not reach either region endpoint. Include cycles, branches,
// missing edges and unreachable nodes without mirroring the two DFS walks.
func TestNativeEnumGeneratedRegionExhaustiveIndependentGraphOracle(t *testing.T) {
	edges := [][2]int{}
	for from := 0; from < 4; from++ {
		for to := 0; to < 4; to++ {
			if from != to {
				edges = append(edges, [2]int{from, to})
			}
		}
	}
	for mask := 0; mask < 1<<len(edges); mask++ {
		ir := &methodir.MethodIR{EntryPC: 0, Instrs: []methodir.Instr{{PC: 3, Opcode: core.OP_RETURN}}}
		var full, cut [4][4]bool
		for i, e := range edges {
			if mask&(1<<i) == 0 {
				continue
			}
			full[e[0]][e[1]] = true
			if e[0] != 2 {
				cut[e[0]][e[1]] = true
			}
			ir.Edges = append(ir.Edges, methodir.Edge{From: methodir.InstrID(e[0]), To: methodir.InstrID(e[1])})
		}
		closure := func(m *[4][4]bool) {
			for k := 0; k < 4; k++ {
				for i := 0; i < 4; i++ {
					for j := 0; j < 4; j++ {
						m[i][j] = m[i][j] || m[i][k] && m[k][j]
					}
				}
			}
		}
		closure(&full)
		closure(&cut)
		want := cut[0][1] && cut[0][2] && !cut[0][3] && !full[2][1] && !full[2][2]
		if got := nativeEnumRegionExecutedOnce(ir, 1, 2, nil); got != want {
			t.Fatalf("graph=%012b got=%v independent=%v", mask, got, want)
		}
	}
}
func TestNativeEnumGeneratedRegionRetainsWorkAndCancellationGuards(t *testing.T) {
	ir := &methodir.MethodIR{EntryPC: 1, Instrs: []methodir.Instr{{PC: 3, Opcode: core.OP_RETURN}}, Edges: []methodir.Edge{{From: 1, To: 2}, {From: 2, To: 3}}}
	for _, variant := range []string{"canceled", "work", "memory"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			limits := workbudget.Limits{}
			if variant == "canceled" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			} else if variant == "work" {
				limits.MaxGraphScans = 1
			} else {
				limits.MaxOutputBytes = 1
			}
			work := workbudget.New(ctx, limits)
			if nativeEnumRegionExecutedOnce(ir, 1, 2, work) || work.Err() == nil {
				t.Fatal("resource refusal must retain original budget error")
			}
		})
	}
}

func TestNativeEnumGeneratedRegionPrecedingConsumerIndependentGraphOracle(t *testing.T) {
	edges := [][2]int{}
	for from := 0; from < 5; from++ {
		for to := from + 1; to < 5; to++ {
			edges = append(edges, [2]int{from, to})
		}
	}
	edges = append(edges, [2]int{3, 1}, [2]int{3, 2})
	for mask := 0; mask < 1<<len(edges); mask++ {
		ir := &methodir.MethodIR{EntryPC: 0, Instrs: []methodir.Instr{{PC: 4, Opcode: core.OP_RETURN}}}
		var full, consumerCut, predecessorCut [5][5]bool
		for i, e := range edges {
			if mask&(1<<i) == 0 {
				continue
			}
			full[e[0]][e[1]] = true
			if e[0] != 3 {
				consumerCut[e[0]][e[1]] = true
			}
			if e[0] != 1 {
				predecessorCut[e[0]][e[1]] = true
			}
			ir.Edges = append(ir.Edges, methodir.Edge{From: methodir.InstrID(e[0]), To: methodir.InstrID(e[1])})
		}
		closure := func(m *[5][5]bool) {
			for k := 0; k < 5; k++ {
				for i := 0; i < 5; i++ {
					for j := 0; j < 5; j++ {
						m[i][j] = m[i][j] || m[i][k] && m[k][j]
					}
				}
			}
		}
		closure(&full)
		closure(&consumerCut)
		closure(&predecessorCut)
		want := consumerCut[0][2] && consumerCut[0][3] && !consumerCut[0][4] && !full[3][2] && !full[3][3] && !predecessorCut[0][2]
		if got := nativeEnumRegionExecutedOnce(ir, 2, 3, nil, 1); got != want {
			t.Fatalf("graph=%012b got=%v independent=%v", mask, got, want)
		}
	}
}
