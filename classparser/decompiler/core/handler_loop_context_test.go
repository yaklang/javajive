package core

import (
	"math/rand"
	"testing"
)

func TestHandlerReentryUsesEnclosingLoopContext(t *testing.T) {
	// Outer header 1, inner header 2. Tail 4 either repeats the inner
	// loop or breaks it; 6 repeats the outer loop. The handler starts
	// at 5 and rejoins tail 4. Re-rooting at 5 invents a second entry
	// into each loop, although every real path first entered header 1.
	succ := [][]int{{1}, {2, 7}, {3, 6}, {4}, {2, 6}, {4}, {1}, {}, {}, {}}
	for _, tc := range []struct {
		name       string
		exceptions [][2]int
		want       bool
	}{
		{"inside_loop", [][2]int{{3, 5}}, true},
		{"outside_loop", [][2]int{{0, 5}}, false},
		{"shared_inside_and_outside", [][2]int{{3, 5}, {0, 5}}, false},
		{"unreachable_throw_site", [][2]int{{9, 5}}, false},
		{"nested_handler", [][2]int{{3, 8}, {8, 5}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := t09CFGFromSuccs(succ, tc.exceptions)
			if err := g.ValidateReducible(); (err == nil) != tc.want {
				t.Fatalf("reducible=%t, error=%v", tc.want, err)
			}
			if want := r07ContextualReducibility(succ, tc.exceptions); want != tc.want {
				t.Fatalf("vertex-deletion oracle=%t, want=%t", want, tc.want)
			}
			if want, _ := t09OracleFromCFG(g); want != tc.want {
				t.Fatalf("iterative-set oracle=%t, want=%t", want, tc.want)
			}
		})
	}
}

func TestHandlerLoopContextAgainstVertexDeletionOracle(t *testing.T) {
	const seed = 2026093001
	rng := rand.New(rand.NewSource(seed))
	for trial := 0; trial < 400; trial++ {
		n := 3 + rng.Intn(10)
		succ := make([][]int, n)
		for u := range succ {
			for edge := 0; edge < 2; edge++ {
				if rng.Intn(3) != 0 {
					succ[u] = append(succ[u], rng.Intn(n))
				}
			}
		}
		exceptions := [][2]int{{rng.Intn(n), rng.Intn(n)}, {rng.Intn(n), rng.Intn(n)}}
		want := r07ContextualReducibility(succ, exceptions)
		g := t09CFGFromSuccs(succ, exceptions)
		if err := g.ValidateReducible(); (err == nil) != want {
			t.Fatalf("seed=%d trial=%d normal=%v exceptions=%v want=%t error=%v", seed, trial, succ, exceptions, want, err)
		}
	}
}
