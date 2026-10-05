package javaclassparser

import (
	"context"
	"slices"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeDependencyObjectsRequireCompleteOwnedBodies(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousStaticDependencyFixture)
	for _, kind := range []string{"original", "nil root", "nil family", "failed family", "wrong root", "nil named", "wrong named identity", "nil anonymous group", "failed anonymous group", "missing anonymous body", "wrong anonymous identity", "duplicate owned identity", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["AnonymousBindingOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			reader := z.nativeMemberReader(root)
			p := reader.planNativeMemberFamily()
			if p == nil || !reader.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original joint ownership")
			}
			const named = "AnonymousBindingOwner$Local"
			const anonymous = "AnonymousBindingOwner$2"
			group := p.anonymousUnits[anonymous]
			if group == nil || group.children[anonymous] == nil {
				t.Fatal("original anonymous declaration")
			}
			var work *workbudget.Budget
			switch kind {
			case "nil root":
				root = nil
			case "nil family":
				p = nil
			case "failed family":
				p.failed = true
			case "wrong root":
				root = p.children[named].object
			case "nil named":
				p.children[named] = nil
			case "wrong named identity":
				p.children[named].object = root
			case "nil anonymous group":
				p.anonymousUnits[anonymous] = nil
			case "failed anonymous group":
				group.failed = true
			case "missing anonymous body":
				delete(group.children, anonymous)
			case "wrong anonymous identity":
				group.children[anonymous].object = root
			case "duplicate owned identity":
				p.children[anonymous] = &nativeMemberClass{object: group.children[anonymous].object}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			objects, known := nativeMemberDependencyObjects(root, p, work)
			if known != (kind == "original") {
				t.Fatalf("admitted=%v", known)
			}
			if !known {
				if objects != nil {
					t.Fatal("partial source unit escaped")
				}
				return
			}
			var names []string
			for _, object := range objects {
				names = append(names, object.GetClassName())
			}
			slices.Sort(names)
			if !slices.Equal(names, []string{"AnonymousBindingOwner", "AnonymousBindingOwner$1", "AnonymousBindingOwner$2", named}) {
				t.Fatalf("incomplete/foreign bodies %v", names)
			}
			rootDependencies, known := nativeMemberDependencyNames(root, nil)
			if !known || slices.Contains(rootDependencies, "ForeignBindingScope$Value") {
				t.Fatal("fixture must isolate a dependency hidden from the enclosing class")
			}
			anonymousDependencies, known := nativeMemberDependencyNames(group.children[anonymous].object, nil)
			if !known || !slices.Contains(anonymousDependencies, "ForeignBindingScope$Value") {
				t.Fatal("missing original anonymous descriptor/Signature edge")
			}
		})
	}
}

func TestNativeAnonymousStaticDependencyDoesNotImportPrivateOwnership(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousStaticDependencyFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["AnonymousBindingOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	entry := z.nativeMemberEntry(root)
	if entry == nil || entry.family == nil {
		t.Fatal("proved family")
	}
	p := entry.family
	if source, known := p.sourceName("ForeignBindingScope$Value"); !known || source != "ForeignBindingScope.Value" {
		t.Fatalf("source alias=%q known=%v", source, known)
	}
	if p.children["ForeignBindingScope$Value"] != nil || p.lexicalObjects["ForeignBindingScope$Value"] != nil {
		t.Fatal("dependency alias granted private lexical ownership")
	}
}
