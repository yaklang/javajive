package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"sync"
	"testing"
)

func TestNativeIndependentLeafGraphKeepsSourceBoundaries(t *testing.T) {
	base := nativeCompileClasses(t, nativeIndependentLeafChainFixture)
	nativeIndependentLeafChainInput(t, base)
	for _, variant := range []string{"original", "ordinary graph", "missing root", "missing dependency", "missing dependency outer", "private root constructor", "farther static boundary", "wrong reciprocal flags", "cycle", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, raw := range base {
				files[n] = append([]byte(nil), raw...)
			}
			root := "LeafNamespace$Leaf"
			var work *workbudget.Budget
			switch variant {
			case "ordinary graph":
				root = "LeafCaller"
			case "missing root":
				delete(files, root+".class")
			case "missing dependency":
				delete(files, "LeafSupport$Base.class")
			case "missing dependency outer":
				delete(files, "LeafSupport.class")
			case "farther static boundary":
				files = nativeCompileClasses(t, "class FartherBase<T>{}class LeafNamespace{static class Middle{static class Leaf<T> extends FartherBase<T>{}}}")
				root = "LeafNamespace$Middle$Leaf"
			case "private root constructor":
				o, _ := Parse(files[root+".class"])
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
				files[root+".class"] = o.Bytes()
			case "wrong reciprocal flags":
				o, _ := Parse(files["LeafSupport.class"])
				for _, a := range o.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(o, row.InnerClassInfoIndex)
							if n == "LeafSupport$Base" {
								row.InnerClassAccessFlags &^= 8
							}
						}
					}
				}
				files["LeafSupport.class"] = o.Bytes()
			case "cycle":
				fixture := strings.Replace(nativeIndependentLeafChainFixture, "class Leaf<T> extends LeafSupport.Base<T>{", "class Leaf<T> extends LeafSupport.Base<T>{LeafSupport.Base<T> peer;", 1)
				fixture = strings.Replace(fixture, "class Base<T> extends LeafBase<T>{", "class Base<T> extends LeafBase<T>{LeafNamespace.Leaf<T> peer;", 1)
				files = nativeCompileClasses(t, fixture)
				nativeIndependentLeafChainInput(t, files)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			z := nativeArchive(t, files)
			defer z.Close()
			graph, known := z.nativeMemberOriginalDependencyGraph(root, work)
			if variant == "original" || variant == "cycle" {
				if !known || len(graph) != 2 || graph["LeafNamespace$Leaf"] == nil || graph["LeafSupport$Base"] == nil || graph["LeafNamespace"] != nil || graph["LeafSupport"] != nil {
					t.Fatalf("leaf dependency source identities: %v %v", graph, known)
				}
				if got := z.nativeMemberDependenciesAcyclic(root, nil); got != (variant == "original") {
					t.Fatalf("original dependency cycle ignored: %v", got)
				}
				if variant == "cycle" {
					obj, _ := Parse(files[root+".class"])
					entry := z.nativeMemberIndependentEntry(obj)
					if entry != nil && entry.family != nil {
						t.Fatal("cyclic leaf publication")
					}
				}
			} else if variant == "ordinary graph" {
				if !known || graph["LeafNamespace"] == nil || graph["LeafSupport"] == nil || graph["LeafNamespace$Leaf"] != nil || graph["LeafSupport$Base"] != nil {
					t.Fatalf("ordinary graph changed physical owners: %v %v", graph, known)
				}
			} else if known {
				t.Fatal("incomplete/foreign/resource graph admitted")
			}
			if variant != "original" && variant != "ordinary graph" && z.sourceOwnership.bytes != 0 {
				t.Fatal("graph query published source")
			}
		})
	}
}
func TestNativeIndependentLeafSourceReadOrderAndConcurrency(t *testing.T) {
	files := nativeCompileClasses(t, nativeIndependentLeafChainFixture)
	nativeIndependentLeafChainInput(t, files)
	var expected map[string][]byte
	for _, order := range [][]string{{"LeafNamespace$Leaf", "LeafSupport$Base"}, {"LeafSupport$Base", "LeafNamespace$Leaf"}} {
		z := nativeArchive(t, files)
		sources := map[string][]byte{}
		for _, name := range order {
			o, _ := Parse(files[name+".class"])
			entry := z.nativeMemberIndependentEntry(o)
			if entry == nil || entry.family == nil {
				t.Fatal("independent dependency read order")
			}
			sources[name] = append([]byte(nil), entry.source...)
		}
		if expected == nil {
			expected = sources
		} else {
			for n, want := range expected {
				if !bytes.Equal(want, sources[n]) {
					t.Fatal("source depends on read order")
				}
			}
		}
		var wait sync.WaitGroup
		results := make([][]byte, 12)
		for i := range results {
			wait.Add(1)
			go func(i int) {
				defer wait.Done()
				name := order[i%2]
				o, _ := Parse(files[name+".class"])
				entry := z.nativeMemberIndependentEntry(o)
				if entry != nil {
					results[i] = []byte(entry.source)
				}
			}(i)
		}
		wait.Wait()
		for i, source := range results {
			if !bytes.Equal(source, sources[order[i%2]]) {
				t.Fatal("concurrent declaration certificate")
			}
		}
		z.Close()
	}
	z := nativeArchive(t, files)
	defer z.Close()
	names := []string{"LeafNamespace$Leaf", "LeafSupport$Base"}
	var wait sync.WaitGroup
	results := make([][]byte, 12)
	for i := range results {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			name := names[i%2]
			o, _ := Parse(files[name+".class"])
			entry := z.nativeMemberIndependentEntry(o)
			if entry != nil {
				results[i] = []byte(entry.source)
			}
		}(i)
	}
	wait.Wait()
	for i, source := range results {
		if !bytes.Equal(source, expected[names[i%2]]) {
			t.Fatal("cold concurrent source publication")
		}
	}
	if z.sourceOwnership.bytes != int64(len(expected[names[0]])+len(expected[names[1]])) {
		t.Fatal("concurrent publication retained duplicate source")
	}
}
