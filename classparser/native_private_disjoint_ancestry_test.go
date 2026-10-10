package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// Complete ancestry may rule out a private target; an unknown/cyclic ancestor,
// same-source interface, shadow declaration or exhausted proof never may.
func TestNativePrivateDisjointAncestryRequiresCompleteNonoverlap(t *testing.T) {
	files := nativeCompileClasses(t, `interface Shared{} interface Left extends Shared{} interface Right extends Shared{} class External implements Left,Right{} class SourceOwner{private int value;}`)
	objects := modernNestTestObjects(t, files)
	source := map[string]*ClassObject{"SourceOwner": objects["SourceOwner"]}
	for _, variant := range []string{"original", "same owner", "nil owned declaration", "interface intersects", "missing parent", "cycle", "wrong object", "nil declaration", "unknown platform", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			parents := map[string][]string{"java/lang/Object": {}, "External": {"java/lang/Object", "Left", "Right"}, "Left": {"Shared"}, "Right": {"Shared"}, "Shared": {}}
			var work *workbudget.Budget
			owner := "External"
			scope := map[string]*ClassObject{}
			for n, o := range source {
				scope[n] = o
			}
			switch variant {
			case "same owner":
				owner = "SourceOwner"
			case "nil owned declaration":
				scope["External"] = nil
			case "interface intersects":
				scope["Shared"] = objects["Shared"]
			case "missing parent":
				delete(parents, "Shared")
			case "cycle":
				parents["Shared"] = []string{"Left"}
			case "unknown platform":
				delete(parents, "java/lang/Object")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			resolve := func(name string) (*ClassObject, bool) {
				if variant == "wrong object" && name == owner {
					return objects["SourceOwner"], true
				}
				if variant == "nil declaration" && name == owner {
					return nil, true
				}
				return nil, false
			}
			fallback := func(name string) ([]string, bool) { v, k := parents[name]; return v, k }
			if got := nativePrivatePermissionDisjointAncestry(owner, scope, resolve, fallback, work); got != (variant == "original") {
				t.Fatalf("disjoint proof %v", got)
			}
		})
	}
	// Actual declarations win even over a contradictory complete fallback.
	if nativePrivatePermissionDisjointAncestry("External", map[string]*ClassObject{"Shared": objects["Shared"]}, func(n string) (*ClassObject, bool) { o := objects[n]; return o, o != nil }, func(string) ([]string, bool) { return nil, true }, nil) {
		t.Fatal("fallback hid original interface")
	}
	for _, width := range []int{255, 256} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			edges := []string{}
			for i := 0; i < width; i++ {
				edges = append(edges, fmt.Sprintf("Leaf%d", i))
			}
			fallback := func(n string) ([]string, bool) {
				if n == "Root" {
					return edges, true
				}
				return nil, true
			}
			if got := nativePrivatePermissionDisjointAncestry("Root", source, func(string) (*ClassObject, bool) { return nil, false }, fallback, nil); got != (width == 255) {
				t.Fatalf("node bound %v", got)
			}
		})
	}
}
func TestNativePrivateDisjointPlatformPreservesCallerAndExactRelease(t *testing.T) {
	files := nativeCompileClasses(t, `class PlatformScope{private int value;}`)
	obj, e := Parse(files["PlatformScope.class"])
	if e != nil {
		t.Fatal(e)
	}
	for _, variant := range []string{"original", "caller sibling", "caller declaration", "unsupported release", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d := NewClassObjectDumper(obj)
			d.options.TargetSourceVersion = 8
			switch variant {
			case "caller sibling":
				d.foldSiblingResolver = func(string) ([]byte, bool) { return []byte{0}, true }
			case "caller declaration":
				d.declarationResolver = func(string) ([]byte, bool) { return nil, true }
			case "unsupported release":
				d.options.TargetSourceVersion = 10
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			parents, known := d.nativePrivatePermissionPlatformParents()("java/util/Collections")
			if known != (variant == "original") {
				t.Fatalf("catalog admission %v %v", known, parents)
			}
		})
	}
}
