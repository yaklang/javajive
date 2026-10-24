package javaclassparser

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Left/Right require a real joint lexical transaction. The controls retain a
// valid post-Java-8 original protocol for Opaque, outside this lexical source
// capability. Its public static Marker can still have an independently proved
// physical declaration spelling. The Marker field is a real reverse type edge,
// not an unused constant-pool entry. A constructor local alone is supported and
// must not be mistaken for that terminal original-protocol refusal.
const nativeCompletionCutFixture = `
class ResidualLeft {
 static int initialized=ResidualDriver.mark("L");
 private Object token;
 ResidualLeft(Object token){this.token=token;}
 class Entry {ResidualRight.Entry peer;Object read(){return token;}}
 Entry make(){return new Entry();}
 OpaqueScope.Marker identity(OpaqueScope.Marker value){return value;}
}
class ResidualRight {
 static int initialized=ResidualDriver.mark("R");
 private Object token;
 ResidualRight(Object token){this.token=token;}
 class Entry {ResidualLeft.Entry peer;Object read(){return token;}}
 Entry make(){return new Entry();}
}
class OpaqueScope {
 public static class Marker {
  ResidualLeft.Entry back;
  Marker(){class ConstructorOnly{}new ConstructorOnly();}
 }
}
class ResidualDriver {
 static String trace="";
 static int mark(String event){trace+=event;return 1;}
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(Object a:new Object[]{null,new Object()})for(Object b:new Object[]{null,a}){
   ResidualLeft left=new ResidualLeft(a);ResidualRight right=new ResidualRight(b);
   ResidualLeft.Entry x=left.make();ResidualRight.Entry y=right.make();x.peer=y;y.peer=x;
   OpaqueScope.Marker marker=new OpaqueScope.Marker();marker.back=x;
   if(left.identity(marker)!=marker||left.identity(null)!=null||marker.back!=x||x.peer.peer!=x||x.read()!=a||y.read()!=b)throw new AssertionError("values and aliases");
   if(x.getClass().getDeclaringClass()!=ResidualLeft.class||y.getClass().getDeclaringClass()!=ResidualRight.class||x.getClass().getDeclaredField("peer").getType()!=ResidualRight.Entry.class||y.getClass().getDeclaredField("peer").getType()!=ResidualLeft.Entry.class)throw new AssertionError("independent lexical owners");
   java.lang.reflect.Field field=ResidualLeft.class.getDeclaredField("token");field.setAccessible(true);field.set(left,b);
   if(x.read()!=b)throw new AssertionError("live private receiver");rows++;
  }
  if(!trace.equals("LR"))throw new AssertionError("initialization order:"+trace);
  System.out.println(rows+":residual:component:physical:dependency:lexical:identity:"+trace);
 }
}`

func TestNativeCompletionDependencySeparatesTerminalNamesFromLexicalParticipants(t *testing.T) {
	files := nativeCompileClasses(t, nativeCompletionCutFixture)
	nativeTestForeignSourceProtocolUnproved(t, files, "OpaqueScope")
	z := nativeArchive(t, files)
	defer z.Close()
	graph, known := z.nativeMemberOriginalDependencyGraph("ResidualLeft", nil)
	if !known || !graph["ResidualLeft"]["OpaqueScope"] || !graph["OpaqueScope"]["ResidualLeft"] {
		t.Fatal("original metadata must retain the real foreign type cycle")
	}
	parts, known := nativeMemberSourceComponents(graph, nil)
	if !known || len(parts["ResidualLeft"]) != 3 {
		t.Fatal("original dependency component", parts["ResidualLeft"])
	}
	participants, cyclic, known := z.nativeMemberDependencyAdmission("ResidualLeft", nil, snapshotJDECEnv())
	if !known || !cyclic || !slices.Equal(participants, []string{"ResidualLeft", "ResidualRight"}) {
		t.Fatalf("completion participants include a separately proved terminal declaration: known=%v cyclic=%v participants=%v", known, cyclic, participants)
	}
	root, err := Parse(files["ResidualLeft.class"])
	if err != nil {
		t.Fatal(err)
	}
	entry := z.nativeMemberTransactionEntry(root)
	if entry == nil || entry.family == nil || entry.source == "" {
		t.Fatal("the two valid lexical families did not complete together")
	}
	if entry.family.children["OpaqueScope$Marker"] != nil || entry.family.lexicalObjects["OpaqueScope$Marker"] != nil || entry.family.allocationDependencies["OpaqueScope$Marker"] != nil {
		t.Fatal("a physical declaration acquired foreign source ownership")
	}
}

func TestNativeCompletionDependencyKeepsUnprovedAndNonstaticEdges(t *testing.T) {
	for _, variant := range []string{"successful foreign plan", "bare cached false", "nonstatic declaration"} {
		t.Run(variant, func(t *testing.T) {
			fixture := nativeCompletionCutFixture
			if variant == "nonstatic declaration" {
				fixture = strings.ReplaceAll(fixture, "public static class Marker", "public class Marker")
				fixture = strings.ReplaceAll(fixture, "new OpaqueScope.Marker()", "new OpaqueScope().new Marker()")
			}
			files := nativeCompileClasses(t, fixture)
			if variant != "successful foreign plan" {
				nativeTestForeignSourceProtocolUnproved(t, files, "OpaqueScope")
			}
			z := nativeArchive(t, files)
			defer z.Close()
			snap := snapshotJDECEnv()
			if variant == "bare cached false" {
				policy, err := json.Marshal(snap)
				if err != nil {
					t.Fatal(err)
				}
				entry := z.nativeMemberPolicyEntry("OpaqueScope\x00" + string(policy))
				entry.planOnce.Do(func() {})
			}
			participants, cyclic, known := z.nativeMemberDependencyAdmission("ResidualLeft", nil, snap)
			if !known || !cyclic || !slices.Equal(participants, []string{"OpaqueScope", "ResidualLeft", "ResidualRight"}) {
				t.Fatalf("a nonterminal or nonstatic declaration cut a dependency: known=%v cyclic=%v participants=%v", known, cyclic, participants)
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(z.nativeMemberTransactions) != 0 {
				t.Fatal("dependency discovery published source ownership")
			}
		})
	}
}

func TestAdversarialCompletionCutKeepsIndependentLexicalCycles(t *testing.T) {
	testCompletionCutRoundTrip(t, false)
}

func TestNativeCompletionDependencyRespectsCurrentPolicyAndResources(t *testing.T) {
	files := nativeCompileClasses(t, nativeCompletionCutFixture)
	nativeTestForeignSourceProtocolUnproved(t, files, "OpaqueScope")
	for _, variant := range []string{"original", "disabled policy", "nil policy", "nil archive", "missing root", "scan limit", "memory limit", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			archive := z
			snapshot := snapshotJDECEnv()
			root := "ResidualLeft"
			var work *workbudget.Budget
			switch variant {
			case "disabled policy":
				snapshot["JDEC_NATIVE_MEMBER_OFF"] = "1"
			case "nil policy":
				snapshot = nil
			case "nil archive":
				archive = nil
			case "missing root":
				root = "AbsentOriginalOwner"
			case "scan limit":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory limit":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := make(map[string][]byte, len(files))
			for name, raw := range files {
				before[name] = bytes.Clone(raw)
			}
			graph, known := archive.nativeMemberCompletionDependencyGraph(root, work, snapshot)
			wantKnown := variant == "original" || variant == "disabled policy"
			if known != wantKnown {
				t.Fatalf("current graph proof=%v", known)
			}
			if known && graph[root]["OpaqueScope"] != (variant == "disabled policy") {
				t.Fatal("a foreign policy granted a terminal type projection")
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(z.nativeMemberTransactions) != 0 {
				t.Fatal("source discovery reserved or published ownership")
			}
			for name, raw := range files {
				if !bytes.Equal(raw, before[name]) {
					t.Fatal("original input mutated", name)
				}
			}
		})
	}
}

func TestAdversarialCompletionCutKeepsIndependentLexicalCyclesJavac8(t *testing.T) {
	testCompletionCutRoundTrip(t, true)
}

// Rebuild both owned lexical families and the separate flat foreign source.
// The original binary caller and constructor-local producer stay unchanged.
// Certify the complete ABI of Left/Right, not the unsupported foreign family's
// lexical metadata: a physical declaration proof grants no such permission.
func testCompletionCutRoundTrip(t *testing.T, native8 bool) {
	t.Helper()
	javac, java := t04Tools(t)
	if native8 {
		javac = os.Getenv("JAVA8_JAVAC")
		if javac == "" {
			t.Fatal("JAVA8_JAVAC required")
		}
	}
	for _, prefix := range []string{"Residual", "Retained"} {
		t.Run(prefix, func(t *testing.T) {
			fixture := strings.ReplaceAll(nativeCompletionCutFixture, "Residual", prefix)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					var files map[string][]byte
					if native8 {
						files = nativePrivateEnumCompile(t, fixture, prefix+"Driver", debug)
					} else {
						files = nativeCompileDebugClasses(t, fixture, debug)
					}
					nativeTestForeignSourceProtocolUnproved(t, files, "OpaqueScope")
					write := func(dir, name string, raw []byte) {
						t.Helper()
						if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					retained := func(name string) bool {
						return name == prefix+"Driver.class" || strings.HasSuffix(name, "$1ConstructorOnly.class")
					}
					var names []string
					original := t.TempDir()
					for name, raw := range files {
						names = append(names, name)
						write(original, name, raw)
					}
					sort.Strings(names)
					const want = "4:residual:component:physical:dependency:lexical:identity:LR\n"
					if got := t04RunJava(t, java, original, prefix+"Driver"); got != want {
						t.Fatal("original independent oracle", got)
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							for _, reverse := range []bool{false, true} {
								t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
									z := nativeTerminalFlatCycleArchive(t, files, native8)
									defer z.Close()
									out := t.TempDir()
									order := slices.Clone(names)
									if reverse {
										slices.Reverse(order)
									}
									var paths []string
									for _, name := range order {
										if retained(name) {
											write(out, name, files[name])
											continue
										}
										source, err := z.ReadFile(name)
										if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.HasPrefix(string(source), "// decompile dump failed") {
											t.Fatalf("source %s: %v\n%s", name, err, source)
										}
										unit := strings.TrimSuffix(name, ".class") + ".java"
										write(out, unit, source)
										paths = append(paths, filepath.Join(out, unit))
									}
									args := []string{"-proc:none", "--release", "8"}
									if native8 {
										args = []string{"-proc:none", "-source", "8", "-target", "8"}
									}
									args = append(args, "-g:"+debug, "-cp", out, "-d", out)
									if log, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
										t.Fatalf("rebuilt source with owned originals absent: %v\n%s", err, log)
									}
									if got := t04RunJava(t, java, out, prefix+"Driver"); got != want {
										t.Fatal("unchanged binary caller", got)
									}
									entries, err := os.ReadDir(out)
									if err != nil {
										t.Fatal(err)
									}
									var inventory []string
									for _, item := range entries {
										if strings.HasSuffix(item.Name(), ".class") {
											inventory = append(inventory, item.Name())
										}
									}
									if !slices.Equal(inventory, names) {
										t.Fatalf("complete binary inventory changed: %v want %v", inventory, names)
									}
									for name, raw := range files {
										got, err := os.ReadFile(filepath.Join(out, name))
										if err != nil {
											t.Fatal(err)
										}
										if retained(name) && !bytes.Equal(got, raw) {
											t.Fatal("binary caller/producer changed", name)
										}
										if strings.HasPrefix(name, prefix+"Left") || strings.HasPrefix(name, prefix+"Right") {
											if nativeBinaryShape(t, raw) != nativeBinaryShape(t, got) || constructorInvocationThrowsDeclarations(t, raw) != constructorInvocationThrowsDeclarations(t, got) {
												t.Fatal("owned full ABI changed", name)
											}
											nativeLexicalExactSignatures(t, name, raw, got)
										}
									}
								})
							}
						})
					}
				})
			}
		})
	}
}
