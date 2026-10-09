package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeStaticDependencyGraphRequiresOriginalClosedOwnership(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticDependencyFixture)
	for _, variant := range []string{"original", "missing root", "missing owned child", "wrong owned self owner", "wrong row flags", "duplicate owned row", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for n, r := range files {
				input[n] = append([]byte(nil), r...)
			}
			var work *workbudget.Budget
			switch variant {
			case "missing root":
				delete(input, "OtherScope.class")
			case "missing owned child":
				delete(input, "OtherScope$Value.class")
			case "wrong owned self owner":
				o, _ := Parse(input["OtherScope$Value.class"])
				for _, a := range o.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(o, row.InnerClassInfoIndex)
							if n == o.GetClassName() {
								row.OuterClassInfoIndex = o.ThisClass
							}
						}
					}
				}
				input["OtherScope$Value.class"] = o.Bytes()
			case "wrong row flags", "duplicate owned row":
				o, _ := Parse(input["OtherScope.class"])
				for _, a := range o.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(o, row.InnerClassInfoIndex)
							if n == "OtherScope$Value" {
								if variant == "wrong row flags" {
									row.InnerClassAccessFlags ^= 8
								} else {
									copy := *row
									table.Classes = append(table.Classes, &copy)
									table.AttrLen += 8
								}
								break
							}
						}
					}
				}
				input["OtherScope.class"] = o.Bytes()
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			z := nativeArchive(t, input)
			defer z.Close()
			if got := z.nativeMemberDependenciesAcyclic("DependencyOwner", work); got != (variant == "original") {
				t.Fatalf("original graph admitted=%v", got)
			}
		})
	}
}

// Two valid original families may refer to each other's static member types.
// They require a joint commit. A one-entry cache must not recurse into itself,
// publish a partial private scope or make ownership depend on read order.
func TestNativeStaticDependencyCyclesRefuseBeforeCacheRecursion(t *testing.T) {
	fixture := strings.ReplaceAll(nativeStaticDependencyFixture, `static class Value{final Object token;`, `static class Value{DependencyOwner.Back unused;final Object token;`)
	fixture = strings.ReplaceAll(fixture, `class DependencyOwner{`, `class DependencyOwner{static class Back{}`)
	files := nativeCompileClasses(t, fixture)
	original := t.TempDir()
	_, java := t04Tools(t)
	for n, r := range files {
		if err := os.WriteFile(filepath.Join(original, n), r, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "DependencyDriver"); got != "static:dependency:identity:callback\n" {
		t.Fatalf("valid cyclic original=%q", got)
	}
	for _, order := range [][]string{{"DependencyOwner", "OtherScope"}, {"OtherScope", "DependencyOwner"}} {
		z := nativeArchive(t, files)
		for _, n := range order {
			o, _ := Parse(append([]byte(nil), files[n+".class"]...))
			if z.nativeMemberDependenciesAcyclic(n, nil) {
				t.Fatal("cycle accepted")
			}
			entry := z.nativeMemberEntry(o)
			if entry != nil && entry.family != nil {
				t.Fatal("partial cyclic family published")
			}
		}
		z.Close()
	}
}

func TestNativeStaticDependencyAliasesDoNotGrantFamilyMembership(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticDependencyFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	o, _ := Parse(files["DependencyOwner.class"])
	entry := z.nativeMemberEntry(o)
	if entry == nil || entry.family == nil {
		t.Fatal("proved independent family")
	}
	p := entry.family
	if _, known := p.sourceName("OtherScope$Value"); !known {
		t.Fatal("missing external static source alias")
	}
	if p.children["OtherScope$Value"] != nil || p.lexicalObjects["OtherScope$Value"] != nil {
		t.Fatal("external alias imported into private lexical ownership")
	}
}

func TestNativeProtectedTypeLexicalConsumersJoinOriginalSourceTransaction(t *testing.T) {
	for _, shape := range []string{"direct", "deep", "shadow"} {
		files := nativeCompileSourceReleaseClasses(t, nativeInheritedContextSources(shape), "none", "8")
		for _, root := range []string{"base/Parent", "use/Owner"} {
			t.Run(shape+"/"+root, func(t *testing.T) {
				z := nativeArchive(t, files)
				defer z.Close()
				graph, known := z.nativeMemberOriginalDependencyGraph(root, nil)
				if !known {
					t.Fatal("original dependency graph unavailable")
				}
				// Direct subclasses already have the protected type scope while flat.
				// A deeper lexical consumer needs its enclosing subclass restored jointly.
				if got := graph["base/Parent"]["use/Owner"]; got != (shape == "deep") {
					t.Fatalf("reverse protected lexical dependency=%v graph=%v", got, graph)
				}
				if root == "use/Owner" || shape == "deep" {
					if !graph["use/Owner"]["base/Parent"] {
						t.Fatal("referencing family lost original declaration dependency")
					}
				}
				components, known := nativeMemberSourceComponents(graph, nil)
				if !known {
					t.Fatal("original source components unavailable")
				}
				if got := len(components[root]) > 1; got != (shape == "deep") {
					t.Fatalf("joint lexical transaction=%v", got)
				}
			})
		}
	}
}
