package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Enumerate all directed graphs on three vertices, including self edges.
// Floyd-Warshall reachability gives an independent component equivalence
// relation, without low-link indices, a DFS stack or the production solver.
func TestNativeSourceComponentsAgainstExhaustiveReachability(t *testing.T) {
	const names = "ABC"
	for sample := 0; sample < 512; sample++ {
		graph := map[string]map[string]bool{}
		reach := [3][3]bool{}
		for i := 0; i < 3; i++ {
			graph[string(names[i])] = map[string]bool{}
			for j := 0; j < 3; j++ {
				if sample&(1<<uint(i*3+j)) != 0 {
					graph[string(names[i])][string(names[j])] = true
					reach[i][j] = true
				}
			}
			reach[i][i] = true
		}
		for k := 0; k < 3; k++ {
			for i := 0; i < 3; i++ {
				for j := 0; j < 3; j++ {
					reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
				}
			}
		}
		components, known := nativeMemberSourceComponents(graph, nil)
		if !known || len(components) != 3 {
			t.Fatalf("sample %d incomplete result", sample)
		}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				found := false
				for _, peer := range components[string(names[i])] {
					found = found || peer == string(names[j])
				}
				if found != (reach[i][j] && reach[j][i]) {
					t.Fatalf("sample=%d pair=%c,%c expected=%v actual=%v", sample, names[i], names[j], reach[i][j] && reach[j][i], found)
				}
			}
		}
	}
}

func TestNativeSourceComponentsRefuseIncompleteOrUnboundedGraph(t *testing.T) {
	for _, variant := range []string{"original", "empty", "unknown target", "false edge", "empty owner", "too many", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			graph := map[string]map[string]bool{"A": {"B": true}, "B": {"A": true}}
			var work *workbudget.Budget
			switch variant {
			case "empty":
				graph = nil
			case "unknown target":
				graph["A"]["missing"] = true
			case "false edge":
				graph["A"]["B"] = false
			case "empty owner":
				graph[""] = nil
			case "too many":
				for i := 0; i < 65; i++ {
					graph[string(rune(0x100+i))] = nil
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if _, known := nativeMemberSourceComponents(graph, work); known != (variant == "original") {
				t.Fatalf("graph admitted=%v", known)
			}
		})
	}
}
