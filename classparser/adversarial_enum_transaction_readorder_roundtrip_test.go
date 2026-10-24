package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The switch-only root and the foreign member interface form a source SCC.
// Suppressing its numbered table is valid only when reading the root selects
// the same committed source transaction, regardless of which entry is read
// first. Neither compilation nor execution may consult original owned classes.
func TestAdversarialEnumTransactionKeepsRootAndArtifactReadOrderConsistent(t *testing.T) {
	const fixture = `enum PacketMode{A,B}
class PacketScope{interface Operation{int apply(PacketMode mode,int value);}static Operation make(){return new PacketRunner();}}
class PacketRunner implements PacketScope.Operation{public int apply(PacketMode mode,int value){switch(mode){case A:return value+7;case B:return value^19;default:throw new AssertionError();}}}
class PacketDriver{public static void main(String[]args)throws Exception{PacketScope.Operation operation=PacketScope.make();int rows=0;for(int repeat=0;repeat<3;repeat++)for(PacketMode mode:PacketMode.values())for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){int expected=mode==PacketMode.A?value+7:value^19;if(operation.apply(mode,value)!=expected)throw new AssertionError("original enum dispatch/overflow");rows++;}try{operation.apply(null,0);throw new AssertionError("null selector must fail");}catch(NullPointerException expected){}Class.forName("PacketRunner$1",false,PacketDriver.class.getClassLoader()).getDeclaredFields();System.out.println(rows+":transaction:table:read-order");}}`
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Packet", "Renamed"} {
		t.Run(prefix, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "Packet", prefix)
			for _, compiler := range []string{"modern", "javac8"} {
				t.Run(compiler, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							var files map[string][]byte
							if compiler == "javac8" {
								files = localGetterCompileJavac8(t, map[string]string{prefix + "Scope.java": source}, debug)
							} else {
								files = nativeCompileDebugClasses(t, source, debug)
							}
							original := t.TempDir()
							for name, raw := range files {
								if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
							want := t04RunJava(t, java, original, prefix+"Driver")
							if want != "30:transaction:table:read-order\n" {
								t.Fatal("original independent oracle", want)
							}
							for _, order := range []string{"root-first", "table-first"} {
								t.Run(order, func(t *testing.T) {
									z := nativeArchive(t, files)
									defer z.Close()
									z.targetSourceVersion = 8
									names := []string{prefix + "Runner.class", prefix + "Runner$1.class"}
									if order == "table-first" {
										names[0], names[1] = names[1], names[0]
									}
									rest := []string{}
									for n := range files {
										if n != prefix+"Driver.class" && n != names[0] && n != names[1] {
											rest = append(rest, n)
										}
									}
									sort.Strings(rest)
									names = append(names, rest...)
									generated := map[string][]byte{}
									for _, n := range names {
										raw, e := z.ReadFile(n)
										if e != nil || strings.Contains(string(raw), DecompileStubMarker) {
											t.Fatalf("source %s: %v\n%s", n, e, raw)
										}
										generated[strings.TrimSuffix(n, ".class")+".java"] = bytes.Clone(raw)
									}
									for _, n := range names {
										again, e := z.ReadFile(n)
										if e != nil || !bytes.Equal(again, generated[strings.TrimSuffix(n, ".class")+".java"]) {
											t.Fatalf("source ownership changed after another entry: %s %v", n, e)
										}
									}
									for _, reverse := range []bool{false, true} {
										label := "ascending"
										if reverse {
											label = "descending"
										}
										t.Run(label, func(t *testing.T) {
											out := t.TempDir()
											if e := os.WriteFile(filepath.Join(out, prefix+"Driver.class"), files[prefix+"Driver.class"], 0600); e != nil {
												t.Fatal(e)
											}
											paths := []string{}
											for n, raw := range generated {
												path := filepath.Join(out, n)
												if e := os.WriteFile(path, raw, 0600); e != nil {
													t.Fatal(e)
												}
												paths = append(paths, path)
											}
											sort.Strings(paths)
											if reverse {
												sort.Sort(sort.Reverse(sort.StringSlice(paths)))
											}
											args := []string{"-proc:none", "--release", "8", "-cp", out, "-d", out}
											if raw, e := exec.Command(javac, append(args, paths...)...).CombinedOutput(); e != nil {
												t.Fatalf("rebuilt whole owned family: %v\n%s\nroot:\n%s", e, raw, generated[prefix+"Runner.java"])
											}
											if got := t04RunJava(t, java, out, prefix+"Driver"); got != want {
												t.Fatalf("candidate=%q want=%q", got, want)
											}
										})
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
