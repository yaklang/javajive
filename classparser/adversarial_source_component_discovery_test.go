package javaclassparser

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Mutual reachability is an independent partition oracle: no DFS indices,
// low-link values, active stack, or transaction-size test contributes to it.
func adversarialComponentReachability(t *testing.T, graph map[string]map[string]bool) {
	t.Helper()
	var names []string
	for name := range graph {
		names = append(names, name)
	}
	slices.Sort(names)
	reach := make([][]bool, len(names))
	for i, name := range names {
		reach[i] = make([]bool, len(names))
		for j, other := range names {
			reach[i][j] = i == j || graph[name][other]
		}
	}
	for k := range names {
		for i := range names {
			for j := range names {
				reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
			}
		}
	}
	components, known := nativeMemberSourceComponents(graph, nil)
	if !known || len(components) != len(graph) {
		t.Fatalf("bounded complete graph lost partition: known=%v owners=%d want=%d", known, len(components), len(graph))
	}
	for i, owner := range names {
		var want []string
		for j, other := range names {
			if reach[i][j] && reach[j][i] {
				want = append(want, other)
			}
		}
		if !slices.Equal(components[owner], want) {
			t.Fatalf("owner=%s component=%v want=%v", owner, components[owner], want)
		}
	}
}

func adversarialComponentGraph(count int, shape, prefix string) map[string]map[string]bool {
	names := make([]string, count)
	graph := map[string]map[string]bool{}
	for i := range names {
		names[i] = fmt.Sprintf("%s%04d", prefix, (i*17+11)%count)
		graph[names[i]] = map[string]bool{}
	}
	for i, name := range names {
		if shape != "chain" || i+1 < count {
			graph[name][names[(i+1)%count]] = true
		}
		if shape == "dense cycle" && i < count/2 {
			for j := count / 2; j < count; j++ {
				graph[name][names[j]] = true
			}
		}
	}
	return graph
}

func TestAdversarialSourceComponentsKeepUnrelatedLargeCyclesKnown(t *testing.T) {
	for _, prefix := range []string{"Scope", "OtherDomain"} {
		for _, count := range []int{63, 64, 65, 97} {
			t.Run(fmt.Sprintf("%s/count=%d", prefix, count), func(t *testing.T) {
				graph := adversarialComponentGraph(count, "cycle", prefix)
				var target string
				for name := range graph {
					target = name
					break
				}
				graph[prefix+"Entry"] = map[string]bool{target: true, prefix + "Left": true}
				graph[prefix+"Left"] = map[string]bool{prefix + "Right": true}
				graph[prefix+"Right"] = map[string]bool{prefix + "Left": true, target: true}
				adversarialComponentReachability(t, graph)
			})
		}
	}
}

func TestAdversarialSourceComponentsMatchReachabilityAcrossTransactionLimit(t *testing.T) {
	for _, prefix := range []string{"Graph", "Renamed"} {
		for _, shape := range []string{"chain", "cycle", "dense cycle"} {
			for _, count := range []int{1, 63, 64, 65, 97} {
				t.Run(fmt.Sprintf("%s/%s/count=%d", prefix, shape, count), func(t *testing.T) {
					adversarialComponentReachability(t, adversarialComponentGraph(count, shape, prefix))
				})
			}
		}
	}
}

func TestAdversarialSourceComponentsKeepDiscoveryResourceBounds(t *testing.T) {
	for _, variant := range []string{"unknown target", "false edge", "nodes", "edges", "node work", "edge work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			graph := adversarialComponentGraph(65, "cycle", "Resource")
			var work *workbudget.Budget
			switch variant {
			case "unknown target":
				graph["Resource0011"]["absent"] = true
			case "false edge":
				graph["Resource0011"]["Resource0011"] = false
			case "nodes":
				graph = adversarialComponentGraph(nativeMemberDependencyNodeLimit+1, "chain", "Resource")
			case "edges":
				graph = map[string]map[string]bool{}
				for i := 0; i < 100; i++ {
					graph[fmt.Sprint("Resource", i)] = map[string]bool{}
				}
				for name, edges := range graph {
					for target := range graph {
						if name != target {
							edges[target] = true
						}
					}
				}
			case "node work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 64})
			case "edge work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 65 + 64})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if components, known := nativeMemberSourceComponents(graph, work); known || components != nil {
				t.Fatal("incomplete or resource-failed discovery returned a partial component certificate")
			}
		})
	}
	// Every vertex/edge is charged twice by the existing discovery contract.
	// Reusing this request budget must not silently create a fresh allowance.
	graph := adversarialComponentGraph(65, "cycle", "Shared")
	work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 2 * (65 + 65)})
	if _, known := nativeMemberSourceComponents(graph, work); !known {
		t.Fatal("exact shared discovery allowance rejected")
	}
	if _, known := nativeMemberSourceComponents(graph, work); known {
		t.Fatal("second discovery reset the shared work budget")
	}
}

func TestAdversarialDependencyAdmissionKeepsOwnTransactionLimit(t *testing.T) {
	for _, prefix := range []string{"Family", "Alternate"} {
		for _, count := range []int{64, 65} {
			t.Run(fmt.Sprintf("%s/count=%d", prefix, count), func(t *testing.T) {
				var source strings.Builder
				fmt.Fprintf(&source, "class %sEntry{static class Item{}%s0.Item input;}class %sLeft{static class Item{%sRight.Item peer;%s0.Item input;}}class %sRight{static class Item{%sLeft.Item peer;}}", prefix, prefix, prefix, prefix, prefix, prefix, prefix)
				for i := 0; i < count; i++ {
					fmt.Fprintf(&source, "class %s%d{static class Item{%s%d.Item peer;}}", prefix, i, prefix, (i+1)%count)
				}
				files := nativeCompileClasses(t, source.String())
				z := nativeArchive(t, files)
				defer z.Close()
				snap := snapshotJDECEnv()
				for _, suffix := range []string{"Entry", "Left", "0"} {
					owner := prefix + suffix
					members, cyclic, known := z.nativeMemberDependencyAdmission(owner, nil, snap)
					if suffix == "0" && count > nativeMemberDependencyComponentLimit {
						if known || cyclic || len(members) != 0 {
							t.Fatal("oversized requested transaction admitted")
						}
						continue
					}
					want := []string{owner}
					if suffix == "Left" {
						want = []string{prefix + "Left", prefix + "Right"}
					}
					if suffix == "0" {
						want = nil
						for i := 0; i < count; i++ {
							want = append(want, fmt.Sprint(prefix, i))
						}
						slices.Sort(want)
					}
					if !known || cyclic != (suffix != "Entry") || !slices.Equal(members, want) {
						t.Fatalf("owner=%s known=%v cyclic=%v component=%v want=%v", owner, known, cyclic, members, want)
					}
				}
				if len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
					t.Fatal("component discovery published source")
				}
				root, err := Parse(files[prefix+"0.class"])
				if err != nil {
					t.Fatal(err)
				}
				entry := z.nativeMemberTransactionEntry(root)
				if count > nativeMemberDependencyComponentLimit {
					if entry != nil || len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
						t.Fatal("oversized SCC bypassed its unchanged transaction publication limit")
					}
				} else if entry == nil || entry.family == nil || entry.source == "" || len(z.nativeMemberTransactions) != 1 || z.nativeMembersBytes == 0 || z.sourceOwnership.bytes != z.nativeMembersBytes {
					t.Fatal("bounded 64-owner transaction failed its normal atomic publication")
				}
			})
		}
	}
}
