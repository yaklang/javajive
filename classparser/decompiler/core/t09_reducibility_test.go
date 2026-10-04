package core

import (
	"strings"
	"testing"
)

type t09Edge struct {
	from, to int
	kind     EdgeKind
}

func t09Graph(n int, edges ...t09Edge) *SemanticCFG {
	g := &SemanticCFG{incoming: map[*OpCode][]int{}, outgoing: map[*OpCode][]int{}}
	for i := 0; i < n; i++ {
		g.Nodes = append(g.Nodes, &OpCode{Id: i, CurrentOffset: uint16(i)})
	}
	for _, e := range edges {
		i := len(g.Edges)
		from, to := g.Nodes[e.from], g.Nodes[e.to]
		g.Edges = append(g.Edges, SemanticEdge{From: from, To: to, Kind: e.kind})
		g.outgoing[from] = append(g.outgoing[from], i)
		g.incoming[to] = append(g.incoming[to], i)
	}
	return g
}

func nextStageGraph(n int, edges ...t09Edge) *SemanticCFG {
	return t09Graph(n, edges...)
}

func TestNextStageT09_C01_NestedIrreducible(t *testing.T) {
	t.Run("T09-C01", testT09C01NestedIrreducible)
}
func testT09C01NestedIrreducible(t *testing.T) {
	// E->H; H->A,B; A->B; B->A,H,X. Maximal SCC H,A,B has one
	// external entry H, but internal cycle A<->B has two entries.
	g := nextStageGraph(5,
		t09Edge{0, 1, EdgeFallthrough}, t09Edge{1, 2, EdgeTaken},
		t09Edge{1, 3, EdgeFallthrough}, t09Edge{2, 3, EdgeTaken},
		t09Edge{3, 2, EdgeTaken}, t09Edge{3, 1, EdgeTaken}, t09Edge{3, 4, EdgeFallthrough})
	err := g.ValidateReducible()
	if err == nil {
		t.Fatal("accepted nested irreducible normal region")
	}
	if !strings.Contains(err.Error(), "unsupported_irreducible_control_flow") {
		t.Fatalf("missing prefix: %v", err)
	}
}

func TestNextStageT09_C02_HandlerIrreducible(t *testing.T) {
	t.Run("T09-C02", testT09C02HandlerIrreducible)
}
func testT09C02HandlerIrreducible(t *testing.T) {
	// The handler H is an exceptional root, not normal-reachable from E.
	// Validate handler NORMAL flow independently; do not treat exception edges
	// themselves as loop backedges.
	g := nextStageGraph(4,
		t09Edge{0, 1, EdgeException}, t09Edge{1, 2, EdgeTaken},
		t09Edge{1, 3, EdgeFallthrough}, t09Edge{2, 3, EdgeTaken}, t09Edge{3, 2, EdgeTaken})
	err := g.ValidateReducible()
	if err == nil {
		t.Fatal("handler normal-flow region was not checked")
	}
	if !strings.Contains(err.Error(), "unsupported_irreducible_control_flow") {
		t.Fatalf("missing prefix: %v", err)
	}
}

func TestNextStageT09_C03_ReducibleControl(t *testing.T) {
	t.Run("T09-C03", testT09C03ReducibleControl)
}
func testT09C03ReducibleControl(t *testing.T) {
	g := nextStageGraph(4,
		t09Edge{0, 1, EdgeFallthrough}, t09Edge{1, 2, EdgeTaken},
		t09Edge{1, 3, EdgeFallthrough}, t09Edge{2, 1, EdgeTaken})
	if err := g.ValidateReducible(); err != nil {
		t.Fatalf("rejected ordinary natural loop: %v", err)
	}
}

func TestT09_C04_ExceptionReentryControl(t *testing.T) {
	t.Run("T09-C04", testT09C04ExceptionReentry)
}
func testT09C04ExceptionReentry(t *testing.T) {
	// Natural loop plus handler with a NORMAL edge into the LOOP HEADER (not body).
	g := nextStageGraph(5,
		t09Edge{0, 1, EdgeFallthrough},
		t09Edge{1, 2, EdgeTaken},
		t09Edge{1, 3, EdgeFallthrough},
		t09Edge{2, 1, EdgeTaken},
		t09Edge{4, 1, EdgeFallthrough},
		t09Edge{2, 4, EdgeException},
	)
	if err := g.ValidateReducible(); err != nil {
		t.Fatalf("flagged ordinary reducible flow because of exception reentry: %v", err)
	}
}

func TestT09_GraphVectors(t *testing.T) {
	type vec struct {
		name      string
		n         int
		edges     []t09Edge
		reducible bool
	}
	cases := []vec{
		{"natural_loop", 4, []t09Edge{
			{0, 1, EdgeFallthrough}, {1, 2, EdgeTaken}, {1, 3, EdgeFallthrough}, {2, 1, EdgeTaken},
		}, true},
		{"nested_irreducible", 5, []t09Edge{
			{0, 1, EdgeFallthrough}, {1, 2, EdgeTaken}, {1, 3, EdgeFallthrough},
			{2, 3, EdgeTaken}, {3, 2, EdgeTaken}, {3, 1, EdgeTaken}, {3, 4, EdgeFallthrough},
		}, false},
		{"handler_irreducible", 4, []t09Edge{
			{0, 1, EdgeException}, {1, 2, EdgeTaken}, {1, 3, EdgeFallthrough},
			{2, 3, EdgeTaken}, {3, 2, EdgeTaken},
		}, false},
		{"exception_reentry_control", 5, []t09Edge{
			{0, 1, EdgeFallthrough}, {1, 2, EdgeTaken}, {1, 3, EdgeFallthrough},
			{2, 1, EdgeTaken}, {4, 1, EdgeFallthrough}, {2, 4, EdgeException},
		}, true},
	}
	for _, tc := range cases {
		g := t09Graph(tc.n, tc.edges...)
		err := g.ValidateReducible()
		if tc.reducible && err != nil {
			t.Errorf("%s: want reducible, got %v", tc.name, err)
		}
		if !tc.reducible && err == nil {
			t.Errorf("%s: want irreducible", tc.name)
		}
		ok, _ := t09OracleFromCFG(g)
		if ok != tc.reducible {
			t.Errorf("%s: oracle reducible=%v want %v", tc.name, ok, tc.reducible)
		}
	}
}
