package javaclassparser

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

const nativeTerminalFlatDependencyFixture = `
class FlagViewRoot {
 private int value;FlagViewRoot(int n){value=n;}
 class Entry {int read(){return value;}}
 Entry entry(){return new Entry();}
 OtherFlagScope.Marker pass(OtherFlagScope.Marker marker){return marker;}
}
class OtherFlagScope {public static class Marker{FlagViewRoot.Entry owner;Marker(){class ConstructorOnly{};new ConstructorOnly();}}}
class FlagViewDriver {public static void main(String[]args)throws Exception{
 int rows=0;

 for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  FlagViewRoot root=new FlagViewRoot(n);FlagViewRoot.Entry entry=root.entry();OtherFlagScope.Marker marker=new OtherFlagScope.Marker();marker.owner=entry;
  if(entry.read()!=n||root.pass(marker)!=marker||root.pass(null)!=null)throw new AssertionError("value or foreign declaration binding");
  java.lang.reflect.Field field=FlagViewRoot.class.getDeclaredField("value");field.setAccessible(true);field.setInt(root,n+17);
  if(entry.read()!=n+17||marker.owner!=entry)throw new AssertionError("live private access or owner identity");rows++;
 }
 System.out.println(rows+":private:accessor:terminal:foreign:identity");
}}
`

func TestAdversarialTerminalFlatDependencyDoesNotBorrowSourceOwnership(t *testing.T) {
	nativeTerminalFlatCycleRoundTrip(t, "", false)
}
func TestAdversarialTerminalFlatCycleRenamedDeclarations(t *testing.T) {
	nativeTerminalFlatCycleRoundTrip(t, "Different", false)
}
func TestAdversarialTerminalFlatCycleOriginalJavac8(t *testing.T) {
	nativeTerminalFlatCycleRoundTrip(t, "", true)
}

// Both source families are rebuilt, while the driver and constructor-local
// producer remain original. Only FlagViewRoot's strict ABI is certified here;
// a flat foreign declaration is not claimed to preserve its lexical metadata.
func nativeTerminalFlatCycleRoundTrip(t *testing.T, prefix string, native8 bool) {
	t.Helper()
	rename := strings.NewReplacer("FlagViewRoot", prefix+"FlagViewRoot", "OtherFlagScope", prefix+"OtherFlagScope", "FlagViewDriver", prefix+"FlagViewDriver")
	source := rename.Replace(nativeTerminalFlatDependencyFixture)
	root, driver := prefix+"FlagViewRoot", prefix+"FlagViewDriver"
	javac, java := t04Tools(t)
	if native8 {
		javac = os.Getenv("JAVA8_JAVAC")
		if javac == "" {
			t.Fatal("JAVA8_JAVAC required")
		}
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			var files map[string][]byte
			if native8 {
				files = nativePrivateEnumCompile(t, source, root, debug)
			} else {
				files = nativeCompileDebugClasses(t, source, debug)
			}
			write := func(root, name string, raw []byte) {
				t.Helper()
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			nativeTestForeignSourceProtocolUnproved(t, files, prefix+"OtherFlagScope")
			original := t.TempDir()
			for n, raw := range files {
				write(original, n, raw)
			}
			const want = "5:private:accessor:terminal:foreign:identity\n"
			if got := t04RunJava(t, java, original, driver); got != want {
				t.Fatal("original", got)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					for _, order := range []string{"forward", "reverse", "failed transaction first"} {
						t.Run(order, func(t *testing.T) {
							z := nativeTerminalFlatCycleArchive(t, files, native8)
							if order == "failed transaction first" {
								o, err := Parse(files[root+".class"])
								if err != nil {
									t.Fatal(err)
								}
								entry := z.nativeMemberTransactionEntry(o)
								if entry != nil && entry.family != nil {
									t.Fatal("terminally refused participant entered transaction")
								}
							}
							defer z.Close()
							out := t.TempDir()
							var paths, names []string
							for n := range files {
								names = append(names, n)
							}
							sort.Strings(names)
							if order == "reverse" {
								sort.Sort(sort.Reverse(sort.StringSlice(names)))
							}
							for _, n := range names {
								raw := files[n]
								if n == driver+".class" || strings.Contains(n, "$1ConstructorOnly.class") {
									write(out, n, raw)
									continue
								}
								source, err := z.ReadFile(n)
								if err != nil || strings.Contains(string(source), DecompileStubMarker) {
									t.Fatalf("source %s:%v\n%s", n, err, source)
								}
								name := strings.TrimSuffix(n, ".class") + ".java"
								write(out, name, source)
								paths = append(paths, filepath.Join(out, name))
							}
							args := []string{"-proc:none", "--release", "8"}
							if native8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							if log, err := exec.Command(javac, append(append(args, "-cp", out, "-d", out), paths...)...).CombinedOutput(); err != nil {
								t.Fatalf("joint source:%v\n%s", err, log)
							}
							if got := t04RunJava(t, java, out, driver); got != want {
								t.Fatal("rebuilt", got)
							}
							for n, raw := range files {
								if n == driver+".class" || strings.Contains(n, "$1ConstructorOnly.class") {
									got, err := os.ReadFile(filepath.Join(out, n))
									if err != nil || !bytes.Equal(raw, got) {
										t.Fatal("original producer/driver changed", n, err)
									}
								}
								if !strings.HasPrefix(n, root) {
									continue
								}
								got, err := os.ReadFile(filepath.Join(out, n))
								if err != nil {
									t.Fatal(err)
								}
								if a, b := nativeBinaryShape(t, raw), nativeBinaryShape(t, got); a != b {
									t.Fatalf("owned ABI %s\n%s\n%s", n, a, b)
								}
								nativeLexicalExactSignatures(t, n, raw, got)
							}
						})
					}
				})
			}
		})
	}
}

func TestNativeAccessorAncestorFlagsNeedOnlyActualSymbolDependencies(t *testing.T) {
	files := nativeCompileClasses(t, nativeTerminalFlatDependencyFixture)
	nativeTestForeignSourceProtocolUnproved(t, files, "OtherFlagScope")
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["FlagViewRoot.class"])
	if err != nil {
		t.Fatal(err)
	}
	prepared := z.prepareNativeMemberFamilyUnpublished(root, snapshotJDECEnv())
	if prepared == nil || len(prepared.family.getters) == 0 {
		t.Fatal("original private getter plan")
	}
	participants, cyclic, known := z.nativeMemberDependencyAdmission("FlagViewRoot", prepared.reader.Work, prepared.snapshot)
	if !known || !cyclic || len(participants) != 2 {
		t.Fatal("real original source cycle", participants, cyclic, known)
	}
	outer, err := Parse(files["OtherFlagScope.class"])
	if err != nil || z.prepareNativeMemberFamilyUnpublished(outer, prepared.snapshot) != nil {
		t.Fatal("foreign local plan must be terminally refused", err)
	}
	resolve := prepared.reader.nativeAnnotationDeclarationResolver()
	for _, getter := range prepared.family.getters {
		matches, known := nativeMemberOriginalAccessorAncestors(prepared.family.lexicalObjects[getter.owner], resolve, prepared.reader.Work)
		if !known || len(matches) != 0 {
			t.Fatal("original ancestor symbol absence", known, len(matches))
		}
	}
	if !z.nativeMemberAccessorAncestorFlagsClosed(prepared, false) {
		t.Fatal("unrelated source dependency cycle changed a closed accessor-flag proof")
	}
}

func TestNativeTerminalFlatDeclarationRequiresNegativeAdmissionWithoutSourceReentry(t *testing.T) {
	for _, variant := range []string{"cold negative", "inside unfinished source completion", "successful local plan", "completed finish refusal", "bare prior refusal", "unpublished peer", "disabled policy", "ambient policy differs", "cache capacity", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			source := nativeTerminalFlatDependencyFixture
			if variant == "successful local plan" {
				source = strings.Replace(source, "Marker(){class ConstructorOnly{};new ConstructorOnly();}", "Marker(){}", 1)
			}
			files := nativeCompileClasses(t, source)
			if variant != "successful local plan" {
				nativeTestForeignSourceProtocolUnproved(t, files, "OtherFlagScope")
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["FlagViewRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := z.prepareNativeMemberFamilyUnpublished(root, snapshotJDECEnv())
			if prepared == nil {
				t.Fatal("own original plan")
			}
			member, err := Parse(files["OtherFlagScope$Marker.class"])
			if err != nil {
				t.Fatal(err)
			}
			policy, _ := json.Marshal(prepared.snapshot)
			entry := z.nativeMemberPolicyEntry("OtherFlagScope\x00" + string(policy))
			if entry == nil {
				t.Fatal("cache entry")
			}
			var peers []map[string]*nativeMemberPrepared
			switch variant {
			case "completed finish refusal":
				entry.planOnce.Do(func() { entry.planKnown = true })
			case "bare prior refusal":
				entry.planOnce.Do(func() { entry.planKnown = false })
			case "unpublished peer":
				peers = []map[string]*nativeMemberPrepared{{"OtherFlagScope": {}}}
			case "disabled policy":
				prepared.snapshot["JDEC_NATIVE_MEMBER_OFF"] = "1"
			case "ambient policy differs":
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			case "cache capacity":
				z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
				for i := 0; i < 512; i++ {
					z.nativeMembersCache[strings.Repeat("x", i+1)] = &nativeMemberCacheEntry{}
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
			got := ""
			query := func() { got = z.terminalFlatStaticDeclaration(prepared, member, peers...) }
			if variant == "inside unfinished source completion" {
				entry.once.Do(query)
			} else {
				query()
			}
			want := ""
			if variant == "cold negative" || variant == "inside unfinished source completion" || variant == "ambient policy differs" {
				want = "OtherFlagScope$Marker"
			}
			if got != want {
				t.Fatalf("terminal declaration=%q want=%q", got, want)
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(prepared.family.sourceDependencies) != 0 || len(prepared.family.allocationDependencies) != 0 || len(prepared.family.children) != 1 {
				t.Fatal("declaration query imported foreign source or ownership")
			}
		})
	}
}

func TestNativeTerminalFlatDeclarationConcurrentColdAdmission(t *testing.T) {
	files := nativeCompileClasses(t, nativeTerminalFlatDependencyFixture)
	nativeTestForeignSourceProtocolUnproved(t, files, "OtherFlagScope")
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(files["FlagViewRoot.class"])
	if err != nil {
		t.Fatal(err)
	}
	p := z.prepareNativeMemberFamilyUnpublished(root, snapshotJDECEnv())
	member, err := Parse(files["OtherFlagScope$Marker.class"])
	if err != nil || p == nil {
		t.Fatal("original declarations", err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; results[i] = z.terminalFlatStaticDeclaration(p, member) }(i)
	}
	close(start)
	wg.Wait()
	for _, got := range results {
		if got != "OtherFlagScope$Marker" {
			t.Fatal("incomplete concurrent declaration", got)
		}
	}
	if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || p.family.children["OtherFlagScope$Marker"] != nil || p.family.lexicalObjects["OtherFlagScope$Marker"] != nil {
		t.Fatal("foreign ownership leaked")
	}
}

func nativeTerminalFlatCycleArchive(t *testing.T, files map[string][]byte, native8 bool) *JarFS {
	t.Helper()
	if !native8 {
		return nativeArchive(t, files)
	}
	jar := filepath.Join(t.TempDir(), "original-native8.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	z, err := NewJarFSFromLocalWithCompilerProfile(jar, 8, NativeJavac8, nil)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

// These tests require a foreign family whose source protocol is unproved.
// A post-SUPER constructor local is now supported and cannot serve as that
// negative precondition. Keep an actual post-Java-8 declaration protocol while
// retaining every instruction, signature and lexical row. Constructor-local
// producer binaries stay at their original version, so javac8 can consume
// those unchanged inputs. Each end-to-end case executes the original first.
func nativeTestForeignSourceProtocolUnproved(t *testing.T, files map[string][]byte, owner string) {
	t.Helper()
	changed := 0
	for name, raw := range files {
		if name != owner+".class" && !strings.HasPrefix(name, owner+"$") {
			continue
		}
		object, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, named := originalMemberOwner(object)
		if name != owner+".class" && !named {
			continue
		}
		if len(raw) < 8 {
			t.Fatal("original class header")
		}
		updated := bytes.Clone(raw)
		updated[6], updated[7] = 0, 55
		if _, err := Parse(updated); err != nil {
			t.Fatal("original version protocol", err)
		}
		files[name] = updated
		changed++
	}
	if changed < 2 {
		t.Fatal("foreign root and named declaration required", owner, changed)
	}
}
