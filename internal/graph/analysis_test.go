package graph

import (
	"reflect"
	"testing"
)

func TestAnalyzeNaturalLoop(t *testing.T) {
	analysis := AnalyzeAdjacency([][]int{
		{1},
		{2, 4},
		{3},
		{1},
		nil,
	}, []int{0})

	assertInts(t, "reachable", analysis.Reachable, []int{0, 1, 2, 3, 4})
	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{1, 2, 3})
	assertInts(t, "entry targets", region.EntryTargets, []int{1})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[int]{{From: 0, To: 1}})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{{From: 1, To: 4}})
}

func TestAnalyzeMultipleLatchLoop(t *testing.T) {
	analysis := AnalyzeAdjacency([][]int{
		{1},
		{2, 3, 4},
		{1},
		{1},
		nil,
	}, []int{0})

	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{1, 2, 3})
	assertInts(t, "entry targets", region.EntryTargets, []int{1})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[int]{{From: 0, To: 1}})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{{From: 1, To: 4}})
}

func TestAnalyzeExcludesUnreachableSecondaryEntry(t *testing.T) {
	analysis := AnalyzeAdjacency([][]int{
		{1},
		{2, 3},
		{1},
		nil,
		{2},
	}, []int{0})

	assertInts(t, "reachable", analysis.Reachable, []int{0, 1, 2, 3})
	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{1, 2})
	assertInts(t, "entry targets", region.EntryTargets, []int{1})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[int]{{From: 0, To: 1}})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{{From: 1, To: 3}})
}

func TestAnalyzeTwoEntrySCC(t *testing.T) {
	analysis := AnalyzeAdjacency([][]int{
		{2, 1},
		{2},
		{1, 3},
		nil,
	}, []int{0})

	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{1, 2})
	assertInts(t, "entry targets", region.EntryTargets, []int{1, 2})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[int]{{From: 0, To: 2}, {From: 0, To: 1}})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{{From: 2, To: 3}})
}

func TestAnalyzeThreeEntrySwitchLikeSCC(t *testing.T) {
	analysis := AnalyzeAdjacency([][]int{
		{3, 1, 2},
		{2, 4},
		{3, 4},
		{1, 4},
		nil,
	}, []int{0})

	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{1, 2, 3})
	assertInts(t, "entry targets", region.EntryTargets, []int{1, 2, 3})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[int]{
		{From: 0, To: 3},
		{From: 0, To: 1},
		{From: 0, To: 2},
	})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{
		{From: 1, To: 4},
		{From: 2, To: 4},
		{From: 3, To: 4},
	})
}

func TestAnalyzeRegionalSCC(t *testing.T) {
	// H, A, and B form one outer SCC with a single external entry at H. The
	// A/B subregion still has two internal entries, H->A and H->B.
	analysis := AnalyzeAdjacency([][]int{
		{1},
		{2, 3},
		{1, 3, 4},
		{2, 1, 4},
		nil,
	}, []int{0})

	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{1, 2, 3})
	assertInts(t, "entry targets", region.EntryTargets, []int{1})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[int]{{From: 0, To: 1}})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{{From: 2, To: 4}, {From: 3, To: 4}})
}

func TestAnalyzeSelfLoop(t *testing.T) {
	analysis := AnalyzeAdjacency([][]int{{0, 1}, nil}, []int{0})

	region := onlyCyclicSCC(t, analysis)
	assertInts(t, "nodes", region.Nodes, []int{0})
	assertInts(t, "entry targets", region.EntryTargets, []int{0})
	assertEdges(t, "entry edges", region.EntryEdges, nil)
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[int]{{From: 0, To: 1}})
}

func TestAnalyzeDeduplicatesNodesAndSuccessors(t *testing.T) {
	analysis := Analyze([]string{"entry", "loop", "loop", "exit"}, []string{"entry"}, func(node string) []string {
		switch node {
		case "entry":
			return []string{"loop", "loop"}
		case "loop":
			return []string{"loop", "exit", "exit"}
		default:
			return nil
		}
	})
	assertStrings(t, "reachable", analysis.Reachable, []string{"entry", "loop", "exit"})
	region := onlyCyclicSCC(t, analysis)
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[string]{{From: "entry", To: "loop"}})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[string]{{From: "loop", To: "exit"}})
}

func TestAnalyzeLargeAcyclicChain(t *testing.T) {
	const size = 20000
	adjacency := make([][]int, size)
	for index := 0; index+1 < size; index++ {
		adjacency[index] = []int{index + 1}
	}
	analysis := AnalyzeAdjacency(adjacency, []int{0})
	if len(analysis.Reachable) != size || len(analysis.SCCs) != size || len(analysis.CyclicSCCs) != 0 {
		t.Fatalf("large chain analysis = reachable %d SCCs %d cyclic %d", len(analysis.Reachable), len(analysis.SCCs), len(analysis.CyclicSCCs))
	}
}

func TestAnalyzeDeterministicStableOrderAndGenericPayload(t *testing.T) {
	nodes := []string{"entry", "gamma", "alpha", "beta", "exit", "dead"}
	successors := map[string][]string{
		"entry": {"beta", "gamma"},
		"gamma": {"exit", "alpha"},
		"alpha": {"beta", "exit"},
		"beta":  {"gamma", "exit"},
		"dead":  {"alpha"},
	}
	analyze := func() Analysis[string] {
		return Analyze(nodes, []string{"entry", "missing", "entry"}, func(node string) []string {
			return successors[node]
		})
	}

	want := analyze()
	for run := 0; run < 20; run++ {
		if got := analyze(); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d differs:\n got: %#v\nwant: %#v", run, got, want)
		}
	}
	if len(want.SCCs) != 3 {
		t.Fatalf("SCC count = %d, want 3: %#v", len(want.SCCs), want.SCCs)
	}
	assertStrings(t, "reachable", want.Reachable, []string{"entry", "gamma", "alpha", "beta", "exit"})
	region := onlyCyclicSCC(t, want)
	assertStrings(t, "nodes", region.Nodes, []string{"gamma", "alpha", "beta"})
	assertStrings(t, "entry targets", region.EntryTargets, []string{"gamma", "beta"})
	assertEdges(t, "entry edges", region.EntryEdges, []Edge[string]{
		{From: "entry", To: "beta"},
		{From: "entry", To: "gamma"},
	})
	assertEdges(t, "exit edges", region.ExitEdges, []Edge[string]{
		{From: "gamma", To: "exit"},
		{From: "alpha", To: "exit"},
		{From: "beta", To: "exit"},
	})
}

func onlyCyclicSCC[T comparable](t *testing.T, analysis Analysis[T]) SCC[T] {
	t.Helper()
	if len(analysis.CyclicSCCs) != 1 {
		t.Fatalf("cyclic SCC count = %d, want 1: %#v", len(analysis.CyclicSCCs), analysis.CyclicSCCs)
	}
	if !analysis.CyclicSCCs[0].Cyclic {
		t.Fatal("cyclic SCC is not marked cyclic")
	}
	return analysis.CyclicSCCs[0]
}

func assertInts(t *testing.T, name string, got, want []int) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertStrings(t *testing.T, name string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertEdges[T comparable](t *testing.T, name string, got, want []Edge[T]) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
}
