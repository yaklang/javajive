package javaclassparser

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Independent transitive closure has no low-link numbering or active stack.
// The expected transaction relation is mutual reachability, across graphs
// larger than the old global cutoff and with wide noncyclic fanout.
func TestNativeWideSourceComponentsAgainstIndependentReachability(t *testing.T) {
	for _, shape := range []string{"chain", "fanout", "blocks", "dense acyclic", "oversized cycle"} {
		t.Run(shape, func(t *testing.T) {
			n := 96
			if shape == "dense acyclic" {
				n = 130
			}
			names := make([]string, n)
			graph := map[string]map[string]bool{}
			reach := make([][]bool, n)
			for i := range names {
				names[i] = fmt.Sprintf("Scope%d", i)
				graph[names[i]] = map[string]bool{}
				reach[i] = make([]bool, n)
				reach[i][i] = true
			}
			add := func(i, j int) { graph[names[i]][names[j]] = true; reach[i][j] = true }
			for i := 0; i < n; i++ {
				switch shape {
				case "chain":
					if i+1 < n {
						add(i, i+1)
					}
				case "fanout":
					if i == 0 {
						for j := 1; j < n; j++ {
							add(i, j)
						}
					}
				case "blocks":
					base := i / 16 * 16
					add(i, base+(i-base+1)%16)
					if i%16 == 0 && i+16 < n {
						add(i, i+16)
					}
				case "dense acyclic":
					if i < n/2 {
						for j := n / 2; j < n; j++ {
							add(i, j)
						}
					}
				case "oversized cycle":
					add(i, (i+1)%n)
				}
			}
			for k := 0; k < n; k++ {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
					}
				}
			}
			components, known := nativeMemberSourceComponents(graph, nil)
			expected := true
			for i := 0; i < n; i++ {
				size := 0
				for j := 0; j < n; j++ {
					if reach[i][j] && reach[j][i] {
						size++
					}
				}
				if size > 64 {
					expected = false
				}
			}
			if known != expected {
				t.Fatalf("complete SCC admission=%v want=%v", known, expected)
			}
			if !known {
				return
			}
			if len(components) != n {
				t.Fatal("incomplete owner map")
			}
			for i := 0; i < n; i++ {
				members := map[string]bool{}
				for _, name := range components[names[i]] {
					if members[name] {
						t.Fatal("duplicate member")
					}
					members[name] = true
				}
				for j := 0; j < n; j++ {
					if members[names[j]] != (reach[i][j] && reach[j][i]) {
						t.Fatalf("partition pair=%d,%d", i, j)
					}
				}
			}
		})
	}
}
func TestNativeWideSourceDependencyGraphAndWarmCacheRespectCurrentResources(t *testing.T) {
	fixture, _ := wideSourceDependencyFixture(96, true)
	files := nativeCompileClasses(t, fixture)
	for _, scenario := range []string{"original", "missing owned class", "foreign self owner", "budget", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			input := map[string][]byte{}
			for n, b := range files {
				input[n] = append([]byte(nil), b...)
			}
			if scenario == "missing owned class" {
				delete(input, "GraphScope70$Marker.class")
			}
			if scenario == "foreign self owner" {
				obj, e := Parse(input["GraphScope70$Marker.class"])
				if e != nil {
					t.Fatal(e)
				}
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == obj.GetClassName() {
								row.OuterClassInfoIndex = obj.ThisClass
							}
						}
					}
				}
				input["GraphScope70$Marker.class"] = obj.Bytes()
			}
			z := nativeArchive(t, input)
			defer z.Close()
			snap := snapshotJDECEnv()
			members, cyclic, known := z.nativeMemberDependencyAdmission("GraphScope0", nil, snap)
			if scenario == "missing owned class" || scenario == "foreign self owner" {
				if known || len(members) != 0 {
					t.Fatal("partial original dependency proof")
				}
				return
			}
			if !known || !cyclic || len(members) != 2 || members[0] != "GraphScope0" || members[1] != "GraphScope1" {
				t.Fatalf("small transaction inside wide graph: %v %v %v", members, cyclic, known)
			}
			var work *workbudget.Budget
			switch scenario {
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("initial allowed unit")
				}
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, _, accepted := z.nativeMemberDependencyAdmission("GraphScope0", work, snap)
			if accepted != (scenario == "original") {
				t.Fatal("cached graph bypassed current work constraints")
			}
		})
	}
}
func TestNativeWideSourceComponentsKeepSeparateGraphAndTransactionLimits(t *testing.T) {
	graph := map[string]map[string]bool{}
	for i := 0; i < nativeMemberDependencyNodeLimit; i++ {
		name := fmt.Sprint("Node", i)
		graph[name] = map[string]bool{}
		if i > 0 {
			graph[fmt.Sprint("Node", i-1)][name] = true
		}
	}
	components, known := nativeMemberSourceComponents(graph, nil)
	if !known || len(components) != nativeMemberDependencyNodeLimit {
		t.Fatal("bounded maximum DAG rejected")
	}
	for n, c := range components {
		if len(c) != 1 || c[0] != n {
			t.Fatal("DAG imported another private scope")
		}
	}
	graph["overflow"] = nil
	if _, known := nativeMemberSourceComponents(graph, nil); known {
		t.Fatal("unbounded node graph accepted")
	}
	dense := map[string]map[string]bool{}
	for i := 0; i < 200; i++ {
		dense[fmt.Sprint("Node", i)] = map[string]bool{}
	}
	for i := 0; i < 100; i++ {
		for j := 100; j < 200; j++ {
			dense[fmt.Sprint("Node", i)][fmt.Sprint("Node", j)] = true
		}
	}
	if _, known := nativeMemberSourceComponents(dense, nil); known {
		t.Fatal("unbounded edge storage accepted")
	}
}
