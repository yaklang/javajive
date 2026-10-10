package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAncestorDeclarationDependencyRequiresOriginalAccessibleHierarchy(t *testing.T) {
	files := nativeCompileClasses(t, nativeInheritedDependencyFixture)
	for _, variant := range []string{"original", "public", "protected", "nil root", "nil member", "nil resolver", "same owner", "unrelated", "missing ancestor", "wrong identity", "interface root", "cycle", "static member", "private member", "wrong self owner", "missing self row", "duplicate self row", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for name, raw := range files {
				o, e := Parse(append([]byte(nil), raw...))
				if e != nil {
					t.Fatal(e)
				}
				objects[strings.TrimSuffix(name, ".class")] = o
			}
			root, member := objects["DependencyDerived"], objects["DependencyBase$Value"]
			resolve := func(name string) (*ClassObject, bool) { o := objects[name]; return o, o != nil }
			var work *workbudget.Budget
			for _, a := range member.Attributes {
				if table, ok := a.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						self, _ := sourceBridgeClassName(member, row.InnerClassInfoIndex)
						if self != member.GetClassName() {
							continue
						}
						switch variant {
						case "public":
							row.InnerClassAccessFlags = 1
						case "protected":
							row.InnerClassAccessFlags = 4
						case "static member":
							row.InnerClassAccessFlags |= 8
						case "private member":
							row.InnerClassAccessFlags |= 2
						case "wrong self owner":
							row.OuterClassInfoIndex = member.ThisClass
						case "missing self row":
							table.Classes = nil
						case "duplicate self row":
							copy := *row
							table.Classes = append(table.Classes, &copy)
						}
						break
					}
				}
			}
			switch variant {
			case "nil root":
				root = nil
			case "nil member":
				member = nil
			case "nil resolver":
				resolve = nil
			case "same owner":
				root = objects["DependencyBase"]
			case "unrelated":
				root = objects["InheritedDependencyDriver"]
			case "missing ancestor":
				delete(objects, "DependencyBase")
			case "wrong identity":
				objects["DependencyBase"] = root
			case "interface root":
				root.AccessFlags |= 0x0200
			case "cycle":
				root.SuperClass = root.ThisClass
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberAncestorDeclarationDependency(root, member, resolve, work); got != (variant == "original" || variant == "public" || variant == "protected") {
				t.Fatalf("ancestor alias admitted=%v", got)
			}
		})
	}
}

func TestNativeAncestorDeclarationAliasKeepsIndependentPrivateScope(t *testing.T) {
	files := nativeCompileClasses(t, nativeInheritedDependencyFixture)
	for _, order := range [][]string{{"DependencyBase", "DependencyDerived"}, {"DependencyDerived", "DependencyBase"}} {
		z := nativeArchive(t, files)
		for _, name := range order {
			obj, _ := Parse(append([]byte(nil), files[name+".class"]...))
			entry := z.nativeMemberEntry(obj)
			if entry == nil || entry.family == nil {
				t.Fatal("complete independent ancestor family")
			}
			if name == "DependencyDerived" {
				p := entry.family
				if p.sourceDependencies["DependencyBase$Value"] == "" || p.children["DependencyBase$Value"] != nil || p.lexicalObjects["DependencyBase$Value"] != nil {
					t.Fatal("source spelling lost or imported ancestor private ownership")
				}
			}
		}
		z.Close()
	}
}

func TestNativeNonAncestorNonstaticDependencyKeepsIndependentOwnership(t *testing.T) {
	f := strings.Replace(nativeInheritedDependencyFixture, "class DependencyDerived extends DependencyBase", "class DependencyDerived", 1)
	f = strings.Replace(f, "final Value value;DependencyDerived(Value value)", "final DependencyBase.Value value;DependencyDerived(DependencyBase.Value value)", 1)
	f = strings.Replace(f, "Object read(Value value)", "Object read(DependencyBase.Value value)", 1)
	files := nativeCompileClasses(t, f)
	dir := t.TempDir()
	_, java := t04Tools(t)
	for name, raw := range files {
		if e := os.WriteFile(filepath.Join(dir, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if got := t04RunJava(t, java, dir, "InheritedDependencyDriver"); got != "2:ancestor:dependency:separate:outer\n" {
		t.Fatalf("valid unrelated original=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	obj, _ := Parse(files["DependencyDerived.class"])
	entry := z.nativeMemberEntry(obj)
	if entry == nil || entry.family == nil {
		t.Fatal("proved independent declaration dependency did not close")
	}
	p := entry.family
	if p.sourceDependencies["DependencyBase$Value"] != "DependencyBase.Value" || p.children["DependencyBase$Value"] != nil || p.lexicalObjects["DependencyBase$Value"] != nil || len(p.constructorBridges("DependencyBase$Value")) != 0 {
		t.Fatal("foreign declaration spelling imported private or lexical ownership")
	}
}

func TestNativeInheritedDeclarationContextRequiresPreparedOriginalScope(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, nativeInheritedContextSources("deep"), "none", "8")
	for _, variant := range []string{"original", "nil prepared", "nil object", "nil member", "nil resolver", "failed family", "wrong root identity", "missing object", "copied object", "missing lexical object", "wrong source spelling", "wrong cached owner", "duplicate owner row", "unrelated enclosing class", "interface enclosing class", "missing ancestor", "wrong ancestor identity", "private member", "static member", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(append([]byte(nil), files["use/Owner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			prepared := z.prepareNativeMemberFamilyUnpublished(root, nil)
			if prepared == nil {
				t.Fatal("valid authored family did not prepare")
			}
			object := prepared.objects["use/Owner$Worker$Reader"]
			member, err := Parse(append([]byte(nil), files["base/Parent$Value.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			originalResolve := prepared.reader.nativeAnnotationDeclarationResolver()
			objects := prepared.objects
			resolve := func(name string) (*ClassObject, bool) {
				if obj := objects[name]; obj != nil {
					return obj, true
				}
				return originalResolve(name)
			}
			worker := prepared.family.children["use/Owner$Worker"]
			var work *workbudget.Budget
			switch variant {
			case "nil prepared":
				prepared = nil
			case "nil object":
				object = nil
			case "nil member":
				member = nil
			case "nil resolver":
				resolve = nil
			case "failed family":
				prepared.family.failed = true
			case "wrong root identity":
				prepared.objects[prepared.family.owner] = object
			case "missing object":
				delete(prepared.objects, object.GetClassName())
			case "copied object":
				copy := *object
				object = &copy
			case "missing lexical object":
				delete(prepared.family.lexicalObjects, worker.object.GetClassName())
			case "wrong source spelling":
				worker.sourceName = "use.Unrelated.Worker"
			case "wrong cached owner":
				worker.owner = worker.object.GetClassName()
			case "duplicate owner row":
				for _, attr := range worker.object.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(worker.object, row.InnerClassInfoIndex)
							if name == worker.object.GetClassName() {
								copy := *row
								table.Classes = append(table.Classes, &copy)
								break
							}
						}
					}
				}
			case "unrelated enclosing class":
				worker.object.SuperClass = 0
			case "interface enclosing class":
				worker.object.AccessFlags |= 0x0200
			case "missing ancestor", "wrong ancestor identity":
				original := resolve
				resolve = func(name string) (*ClassObject, bool) {
					if name == "base/Parent" {
						if variant == "missing ancestor" {
							return nil, false
						}
						return root, true
					}
					return original(name)
				}
			case "private member", "static member":
				for _, attr := range member.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(member, row.InnerClassInfoIndex)
							if name == member.GetClassName() {
								if variant == "private member" {
									row.InnerClassAccessFlags = 2
								} else {
									row.InnerClassAccessFlags |= 8
								}
							}
						}
					}
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
			if got := nativeMemberLexicalAncestorDeclarationDependency(prepared, object, member, resolve, work); got != (variant == "original") {
				t.Fatalf("original lexical type scope admitted=%v", got)
			}
		})
	}
}
