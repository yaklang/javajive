package javaclassparser

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeFlatStaticDeclarationConcurrentViewsOwnNoForeignSource(t *testing.T) {
	files := compileFlatStaticDeclarationSources(t, flatStaticDeclarationSources("enum"), "none", "11")
	archive := nativeArchive(t, files)
	defer archive.Close()
	archive.targetSourceVersion = 8
	archive.sourceCompiler = ModernJavac
	root, err := Parse(files["q/Consumer.class"])
	if err != nil {
		t.Fatal(err)
	}
	prepared := archive.prepareNativeMemberFamilyFromRoot(root, snapshotJDECEnv(), nil)
	if prepared == nil {
		t.Fatal("original consumer plan")
	}
	member, err := Parse(files["p/ForeignOwner$Choice.class"])
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make([]string, 8)
	var readers sync.WaitGroup
	for i := range results {
		readers.Add(1)
		go func(i int) {
			defer readers.Done()
			<-start
			results[i] = archive.completedFlatStaticDeclaration(prepared, member)
		}(i)
	}
	close(start)
	readers.Wait()
	for _, result := range results {
		if result != "p.ForeignOwner$Choice" {
			t.Fatalf("incomplete concurrent declaration view: %q", result)
		}
	}
	if archive.nativeMembersBytes != 0 || archive.sourceOwnership.bytes != 0 || prepared.family.children["p/ForeignOwner$Choice"] != nil || prepared.family.lexicalObjects["p/ForeignOwner$Choice"] != nil || prepared.family.allocationDependencies["p/ForeignOwner$Choice"] != nil {
		t.Fatal("a declaration view reserved or imported foreign source ownership")
	}
}

func TestNativeFlatStaticDeclarationRequiresTerminalOriginalNamespace(t *testing.T) {
	files := compileFlatStaticDeclarationSources(t, flatStaticDeclarationSources("enum"), "none", "11")
	for _, variant := range []string{"original", "nil declaration", "nil plan", "missing family", "failed family", "missing reader", "missing snapshot", "same owner", "nonstatic", "private", "protected", "nonpublic physical class", "duplicate self", "wrong simple name", "missing original", "wrong resolved identity", "missing outer", "unreciprocated outer", "unpublished peer", "disabled source policy", "ambient policy differs", "lookup capacity", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			archive.targetSourceVersion = 8
			archive.sourceCompiler = ModernJavac
			root, err := Parse(files["q/Consumer.class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := archive.prepareNativeMemberFamilyFromRoot(root, snapshotJDECEnv(), nil)
			if prepared == nil {
				t.Fatal("independent original consumer plan")
			}
			member, err := Parse(files["p/ForeignOwner$Choice.class"])
			if err != nil {
				t.Fatal(err)
			}
			var peers []map[string]*nativeMemberPrepared
			mutateSelf := func(change func(*InnerClassInfo)) {
				t.Helper()
				for _, attribute := range member.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, known := sourceBridgeClassName(member, row.InnerClassInfoIndex)
							if known && n == member.GetClassName() {
								change(row)
								return
							}
						}
					}
				}
				t.Fatal("independent original self row missing")
			}
			resolver := prepared.reader.foldSiblingResolver
			switch variant {
			case "nil declaration":
				member = nil
			case "nil plan":
				prepared = nil
			case "missing family":
				prepared.family = nil
			case "failed family":
				prepared.family.failed = true
			case "missing reader":
				prepared.reader = nil
			case "missing snapshot":
				prepared.snapshot = nil
			case "same owner":
				member, err = Parse(files["q/Consumer$Member.class"])
				if err != nil {
					t.Fatal(err)
				}
			case "nonstatic":
				mutateSelf(func(row *InnerClassInfo) { row.InnerClassAccessFlags &^= 8 })
			case "private":
				mutateSelf(func(row *InnerClassInfo) { row.InnerClassAccessFlags = row.InnerClassAccessFlags&^1 | 2 })
			case "protected":
				mutateSelf(func(row *InnerClassInfo) { row.InnerClassAccessFlags = row.InnerClassAccessFlags&^1 | 4 })
			case "nonpublic physical class":
				member.AccessFlags &^= 1
			case "wrong simple name":
				mutateSelf(func(row *InnerClassInfo) {
					member.ConstantPool[row.InnerNameIndex-1].(*ConstantUtf8Info).Value = "Other"
				})
			case "duplicate self":
				for _, attribute := range member.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(member, row.InnerClassInfoIndex)
							if n == member.GetClassName() {
								copy := *row
								table.Classes = append(table.Classes, &copy)
								break
							}
						}
					}
				}
			case "missing original":
				prepared.reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "p/ForeignOwner$Choice" {
						return nil, false
					}
					return resolver(name)
				}
			case "wrong resolved identity":
				prepared.reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "p/ForeignOwner$Choice" {
						return files["q/Consumer.class"], true
					}
					return resolver(name)
				}
			case "missing outer":
				prepared.reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "p/ForeignOwner" {
						return nil, false
					}
					return resolver(name)
				}
			case "unreciprocated outer":
				outer, err := Parse(files["p/ForeignOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				for _, attribute := range outer.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(outer, row.InnerClassInfoIndex)
							if n == "p/ForeignOwner$Choice" {
								row.InnerClassAccessFlags ^= 16
							}
						}
					}
				}
				bytes := outer.Bytes()
				prepared.reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "p/ForeignOwner" {
						return bytes, true
					}
					return resolver(name)
				}
			case "unpublished peer":
				peers = []map[string]*nativeMemberPrepared{{"p/ForeignOwner": {}}}
			case "disabled source policy":
				prepared.snapshot = map[string]string{"JDEC_NATIVE_MEMBER_OFF": "1"}
			case "ambient policy differs":
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			case "lookup capacity":
				archive.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
				for i := 0; i < 512; i++ {
					archive.nativeMembersCache[strings.Repeat("x", i+1)] = &nativeMemberCacheEntry{}
				}
			case "budget":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				prepared.reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := archive.completedFlatStaticDeclaration(prepared, member, peers...)
			want := ""
			if variant == "original" || variant == "ambient policy differs" {
				want = "p.ForeignOwner$Choice"
			}
			if got != want {
				t.Fatalf("declaration spelling=%q want=%q", got, want)
			}
		})
	}
}

func TestNativeFlatStaticDeclarationDoesNotTreatCompletedPlanAsFlatEvidence(t *testing.T) {
	files := compileFlatStaticDeclarationSources(t, flatStaticDeclarationSources("enum"), "none", "8")
	archive := nativeArchive(t, files)
	defer archive.Close()
	archive.targetSourceVersion = 8
	archive.sourceCompiler = ModernJavac
	root, err := Parse(files["q/Consumer.class"])
	if err != nil {
		t.Fatal(err)
	}
	prepared := archive.prepareNativeMemberFamilyFromRoot(root, snapshotJDECEnv(), nil)
	if prepared == nil {
		t.Fatal("original consumer plan")
	}
	member, err := Parse(files["p/ForeignOwner$Choice.class"])
	if err != nil {
		t.Fatal(err)
	}
	if source := archive.completedFlatStaticDeclaration(prepared, member); source != "" {
		t.Fatalf("a successful local plan falsely granted a flat spelling: %s", source)
	}
	outer, err := Parse(files["p/ForeignOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	entry := archive.nativeMemberEntry(outer)
	if entry == nil || !entry.planKnown {
		t.Fatal("independent original outer should have a successful local plan")
	}
}

func TestNativeFlatStaticDeclarationRetainsPhysicalPackageAccess(t *testing.T) {
	sources := flatStaticDeclarationSources("package enum")
	sources["q/Unrelated.java"] = `package q;public class Unrelated{public static class Marker{}}`
	files := compileFlatStaticDeclarationSources(t, sources, "none", "11")
	for _, actor := range []string{"p/Consumer", "q/Unrelated"} {
		t.Run(actor, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			archive.targetSourceVersion = 8
			archive.sourceCompiler = ModernJavac
			root, err := Parse(files[actor+".class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := archive.prepareNativeMemberFamilyFromRoot(root, snapshotJDECEnv(), nil)
			if prepared == nil {
				t.Fatal("original referencing family")
			}
			member, err := Parse(files["p/ForeignOwner$Choice.class"])
			if err != nil {
				t.Fatal(err)
			}
			if member.AccessFlags&1 != 0 {
				t.Fatal("fixture must exercise an originally nonpublic ClassFile")
			}
			got := archive.completedFlatStaticDeclaration(prepared, member)
			want := ""
			if actor == "p/Consumer" {
				want = "p.ForeignOwner$Choice"
			}
			if got != want {
				t.Fatalf("physical package access %s: %q want %q", actor, got, want)
			}
		})
	}
}

func TestNativeFlatStaticDeclarationRejectsContradictoryOriginalKind(t *testing.T) {
	original := compileFlatStaticDeclarationSources(t, flatStaticDeclarationSources("enum"), "none", "11")
	files := map[string][]byte{}
	for name, raw := range original {
		files[name] = raw
	}
	for _, name := range []string{"p/ForeignOwner", "p/ForeignOwner$Choice"} {
		object, err := Parse(original[name+".class"])
		if err != nil {
			t.Fatal(err)
		}
		for _, attribute := range object.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				for _, row := range table.Classes {
					binary, _ := sourceBridgeClassName(object, row.InnerClassInfoIndex)
					if binary == "p/ForeignOwner$Choice" {
						row.InnerClassAccessFlags &^= 0x4000
					}
				}
			}
		}
		files[name+".class"] = object.Bytes()
	}
	archive := nativeArchive(t, files)
	defer archive.Close()
	archive.targetSourceVersion = 8
	archive.sourceCompiler = ModernJavac
	root, err := Parse(files["q/Consumer.class"])
	if err != nil {
		t.Fatal(err)
	}
	prepared := archive.prepareNativeMemberFamilyFromRoot(root, snapshotJDECEnv(), nil)
	if prepared == nil {
		t.Fatal("original consumer plan")
	}
	member, err := Parse(files["p/ForeignOwner$Choice.class"])
	if err != nil {
		t.Fatal(err)
	}
	if got := archive.completedFlatStaticDeclaration(prepared, member); got != "" {
		t.Fatalf("reciprocal rows disagreed with the physical declaration kind: %s", got)
	}
}
