package core

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

func naiveNearestConditions(sources []*OpCode) []*OpCode {
	var out []*OpCode
	for _, source := range sources {
		_ = WalkGraph[*OpCode](source, func(code *OpCode) ([]*OpCode, error) {
			if isConditionOpcode(code) {
				if !slices.Contains(out, code) {
					out = append(out, code)
				}
				return nil, nil
			}
			return code.Source, nil
		})
	}
	return out
}

func conditionGraphTestNodes(count int) []*OpCode {
	nodes := make([]*OpCode, count)
	for index := range nodes {
		opcode := OP_NOP
		if index%3 == 1 {
			opcode = OP_IFEQ
		}
		nodes[index] = &OpCode{Instr: InstrInfos[opcode], CurrentOffset: uint16(index), Id: index}
	}
	return nodes
}

func TestMergeConditionIndexMatchesWalkGraph(t *testing.T) {
	// Linear chain with a condition every third node.
	chain := conditionGraphTestNodes(40)
	for index := 1; index < len(chain); index++ {
		chain[index].Source = []*OpCode{chain[index-1]}
	}
	index := newMergeConditionIndex()
	for _, node := range chain {
		got := index.nearest(node)
		want := naiveNearestConditions([]*OpCode{node})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("node %d nearest = %v, want %v", node.Id, conditionIDs(got), conditionIDs(want))
		}
	}

	// Diamond: condition -> two arms -> merge. The condition is nearest to both arms.
	cond := &OpCode{Instr: InstrInfos[OP_IFEQ], Id: 100}
	left := &OpCode{Instr: InstrInfos[OP_NOP], Id: 101, Source: []*OpCode{cond}}
	right := &OpCode{Instr: InstrInfos[OP_NOP], Id: 102, Source: []*OpCode{cond}}
	merge := &OpCode{Instr: InstrInfos[OP_NOP], Id: 103, Source: []*OpCode{left, right}}
	if got, want := index.collect(merge.Source), naiveNearestConditions(merge.Source); !reflect.DeepEqual(got, want) {
		t.Fatalf("diamond conditions = %v, want %v", conditionIDs(got), conditionIDs(want))
	}

	// Nested condition: the inner condition owns the merge; the outer one is
	// behind it and must not be reported.
	outer := &OpCode{Instr: InstrInfos[OP_IFEQ], Id: 200}
	inner := &OpCode{Instr: InstrInfos[OP_IFEQ], Id: 201, Source: []*OpCode{outer}}
	tail := &OpCode{Instr: InstrInfos[OP_NOP], Id: 202, Source: []*OpCode{inner}}
	if got, want := index.collect([]*OpCode{tail}), naiveNearestConditions([]*OpCode{tail}); !reflect.DeepEqual(got, want) {
		t.Fatalf("nested conditions = %v, want %v", conditionIDs(got), conditionIDs(want))
	}

	// A cycle without conditions contributes nothing, and must terminate.
	first := &OpCode{Instr: InstrInfos[OP_NOP], Id: 300}
	second := &OpCode{Instr: InstrInfos[OP_NOP], Id: 301, Source: []*OpCode{first}}
	first.Source = []*OpCode{second}
	if got, want := index.collect([]*OpCode{first}), naiveNearestConditions([]*OpCode{first}); !reflect.DeepEqual(got, want) {
		t.Fatalf("cyclic conditions = %v, want %v", conditionIDs(got), conditionIDs(want))
	}
}

func conditionIDs(nodes []*OpCode) []int {
	out := make([]int, len(nodes))
	for index, node := range nodes {
		out[index] = node.Id
	}
	return out
}

func TestMergeConditionIndexRandomDAGDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	for trial := 0; trial < 40; trial++ {
		n := 80
		nodes := make([]*OpCode, n)
		for index := range nodes {
			opcode := OP_NOP
			if index%4 == 0 {
				opcode = OP_IFEQ
			}
			nodes[index] = &OpCode{Instr: InstrInfos[opcode], CurrentOffset: uint16(index), Id: index}
		}
		for index := 1; index < n; index++ {
			// Acyclic by construction: sources always have smaller indexes.
			for source := 0; source < index; source++ {
				if rng.Intn(5) == 0 {
					nodes[index].Source = append(nodes[index].Source, nodes[source])
				}
			}
		}
		index := newMergeConditionIndex()
		for _, node := range nodes {
			got := index.nearest(node)
			want := naiveNearestConditions([]*OpCode{node})
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("trial %d node %d nearest = %v, want %v", trial, node.Id, conditionIDs(got), conditionIDs(want))
			}
		}
	}
}

func TestMergeConditionIndexDeepChainTerminates(t *testing.T) {
	const depth = 20000
	nodes := make([]*OpCode, depth)
	nodes[0] = &OpCode{Instr: InstrInfos[OP_IFEQ], Id: 0}
	for index := 1; index < depth; index++ {
		nodes[index] = &OpCode{Instr: InstrInfos[OP_NOP], Id: index, Source: []*OpCode{nodes[index-1]}}
	}
	index := newMergeConditionIndex()
	got := index.collect([]*OpCode{nodes[depth-1]})
	if len(got) != 1 || got[0] != nodes[0] {
		t.Fatalf("deep chain nearest = %v", conditionIDs(got))
	}
}

func BenchmarkMergeConditionIndexChain(b *testing.B) {
	for _, depth := range []int{512, 4096, 16384} {
		b.Run(fmt.Sprintf("%d", depth), func(b *testing.B) {
			nodes := make([]*OpCode, depth)
			nodes[0] = &OpCode{Instr: InstrInfos[OP_IFEQ], Id: 0}
			for index := 1; index < depth; index++ {
				nodes[index] = &OpCode{Instr: InstrInfos[OP_NOP], Id: index, Source: []*OpCode{nodes[index-1]}}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				index := newMergeConditionIndex()
				got := index.collect([]*OpCode{nodes[depth-1]})
				if len(got) != 1 {
					b.Fatalf("conditions = %v", conditionIDs(got))
				}
			}
		})
	}
}
