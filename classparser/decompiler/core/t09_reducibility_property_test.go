package core

import (
	"math/rand"
	"strings"
	"testing"
)

func t09UniqueSuccs(n int, raw [][]int) [][]int {
	out := make([][]int, n)
	for u := 0; u < n; u++ {
		seen := map[int]struct{}{}
		for _, v := range raw[u] {
			if v < 0 || v >= n {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out[u] = append(out[u], v)
		}
	}
	return out
}

func t09CFGFromSuccs(succs [][]int, exception [][2]int) *SemanticCFG {
	n := len(succs)
	edges := make([]t09Edge, 0, n*2)
	for u := 0; u < n; u++ {
		for _, v := range succs[u] {
			kind := EdgeFallthrough
			if u == v || v < u {
				kind = EdgeTaken
			}
			edges = append(edges, t09Edge{u, v, kind})
		}
	}
	for _, e := range exception {
		edges = append(edges, t09Edge{e[0], e[1], EdgeException})
	}
	return t09Graph(n, edges...)
}

func TestT09_C05_PropertyVsIndependentOracle(t *testing.T) {
	t.Run("T09-C05", testT09C05Property)
}
func testT09C05Property(t *testing.T) {
	selfLoop := t09CFGFromSuccs([][]int{{0}}, nil)
	if err := selfLoop.ValidateReducible(); err != nil {
		t.Fatalf("self-loop rejected: %v", err)
	}
	if ok, _ := t09OracleFromCFG(selfLoop); !ok {
		t.Fatal("oracle rejected self-loop")
	}

	nested := t09CFGFromSuccs([][]int{
		{1},
		{2, 4},
		{3, 1},
		{2},
		{},
	}, nil)
	if err := nested.ValidateReducible(); err != nil {
		t.Fatalf("nested natural loops rejected: %v", err)
	}

	chainN := 40
	chain := make([][]int, chainN)
	for i := 0; i < chainN-1; i++ {
		chain[i] = []int{i + 1}
	}
	long := t09CFGFromSuccs(chain, nil)
	if err := long.ValidateReducible(); err != nil {
		t.Fatalf("long chain rejected: %v", err)
	}

	rng := rand.New(rand.NewSource(20260921))
	for n := 2; n <= 9; n++ {
		for trial := 0; trial < 24; trial++ {
			raw := make([][]int, n)
			for i := 0; i < n; i++ {
				if i+1 < n && rng.Intn(4) != 0 {
					raw[i] = append(raw[i], i+1)
				}
				degree := rng.Intn(3)
				for k := 0; k < degree; k++ {
					raw[i] = append(raw[i], rng.Intn(n))
				}
			}
			succs := t09UniqueSuccs(n, raw)
			var ex [][2]int
			if rng.Intn(3) == 0 {
				from := rng.Intn(n)
				to := rng.Intn(n)
				ex = [][2]int{{from, to}}
			}
			g := t09CFGFromSuccs(succs, ex)
			var first string
			for i := 0; i < 10; i++ {
				err := g.ValidateReducible()
				msg := ""
				if err != nil {
					msg = err.Error()
				}
				if i == 0 {
					first = msg
				} else if msg != first {
					t.Fatalf("n=%d trial=%d unstable diagnostic %q vs %q", n, trial, first, msg)
				}
			}
			prodErr := g.ValidateReducible()
			ok, _ := t09OracleFromCFG(g)
			if ok != (prodErr == nil) {
				t.Fatalf("n=%d trial=%d production err=%v oracle reducible=%v succs=%v ex=%v", n, trial, prodErr, ok, succs, ex)
			}
			if prodErr != nil && !strings.Contains(prodErr.Error(), "unsupported_irreducible_control_flow") {
				t.Fatalf("missing prefix: %v", prodErr)
			}
		}
	}
}
