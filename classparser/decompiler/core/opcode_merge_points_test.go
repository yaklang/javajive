package core

import (
	"math/rand"
	"testing"
)

func mergeFixture(succ [][]int) []*OpCode {
	nodes := make([]*OpCode, len(succ))
	for i := range nodes {
		// Id and PC ordering are deliberately unrelated to graph distance.
		nodes[i] = &OpCode{Id: len(succ) - i, CurrentOffset: uint16(10 * i)}
	}
	for i, targets := range succ {
		for _, j := range targets {
			LinkOpcode(nodes[i], nodes[j])
		}
	}
	return nodes
}

func TestOpcodeMergePointIsNearestPostdominatorInsideLoop(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		succ := [][]int{{1, 6}, {2, 3}, {4}, {4}, {5}, {0}, nil}
		if reverse {
			succ[1][0], succ[1][1] = succ[1][1], succ[1][0]
		}
		nodes := mergeFixture(succ)
		if got := OpcodeMergePoints(nodes, nodes[0])[nodes[1]]; got != nodes[4] {
			t.Fatalf("reverse=%t: picked loop header or farther continuation: %p", reverse, got)
		}
		if got := CalcMergeOpcode(nodes[1]); got != nodes[4] {
			t.Fatal("standalone merge query disagrees")
		}
	}
}

// Independently construct the reverse graph, then delete each vertex to test
// dominance. No production postdominator, exit reachability or idom helpers
// are used to decide the expected join.
func mergeOracle(succ [][]int) [][]bool {
	n := len(succ)
	live := r07Reach(succ, 0, -1)
	canExit := make([]bool, n)
	for i := range succ {
		if !live[i] {
			continue
		}
		reach := r07Reach(succ, i, -1)
		for j := range succ {
			canExit[i] = canExit[i] || reach[j] && len(succ[j]) == 0
		}
	}
	reverse := make([][]int, n+1)
	for i, targets := range succ {
		if !live[i] {
			continue
		}
		for _, j := range targets {
			reverse[j] = append(reverse[j], i)
		}
		if len(targets) == 0 || !canExit[i] {
			reverse[n] = append(reverse[n], i)
		}
	}
	return r07Dominance(reverse, n)
}

func TestOpcodeMergePointsAgainstIndependentDeletionOracle(t *testing.T) {
	random := rand.New(rand.NewSource(0x4d45524745))
	for trial := 0; trial < 1000; trial++ {
		n := 2 + random.Intn(9)
		succ := make([][]int, n)
		for i := range succ {
			for edges := random.Intn(3); edges > 0; edges-- {
				succ[i] = append(succ[i], random.Intn(n))
			}
		}
		nodes := mergeFixture(succ)
		got := OpcodeMergePoints(nodes, nodes[0])
		dom := mergeOracle(succ)
		live := r07Reach(succ, 0, -1)
		for i := range succ {
			if len(nodes[i].Target) != 2 {
				continue
			}
			var want *OpCode
			if live[i] {
				for candidate := 0; candidate < n; candidate++ {
					if candidate == i || !dom[candidate][i] {
						continue
					}
					nearest := true
					for other := 0; other < n; other++ {
						if other != i && other != candidate && dom[other][i] && dom[candidate][other] {
							nearest = false
						}
					}
					if nearest {
						want = nodes[candidate]
					}
				}
			}
			if got[nodes[i]] != want {
				t.Fatalf("trial=%d node=%d succ=%v got=%p want=%p", trial, i, succ, got[nodes[i]], want)
			}
		}
	}
}
