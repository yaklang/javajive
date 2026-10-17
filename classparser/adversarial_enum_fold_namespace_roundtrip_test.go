package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func enumFoldNamespaceFixture(variant string) (string, string) {
	root := "EnumFoldSpace"
	source := `class EnumFoldSpace {
 public enum Mode { A,B }
 public static final EnumFoldOp OP=new EnumFoldOp(){public int apply(int value){return value+3;}};
 public static int choose(Mode mode,int value){switch(mode){case A:return OP.apply(value);case B:return -value;default:throw new AssertionError();}}
}
interface EnumFoldOp {int apply(int value);}
class EnumFoldTrace {static int value;}
class EnumFoldDriver {public static void main(String[]args){
 EnumFoldTrace.value=0;
	try{EnumFoldSpace.choose(null,0);throw new AssertionError("null selector must fail");}catch(NullPointerException expected){}
 for(int repeat=0;repeat<3;repeat++)for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(EnumFoldSpace.Mode mode:EnumFoldSpace.Mode.values()){
  int expected=mode==EnumFoldSpace.Mode.A?value+3:-value;
  if(EnumFoldSpace.choose(mode,value)!=expected)throw new AssertionError("enum dispatch/overflow");
 }
 if(EnumFoldTrace.value!=TRACE)throw new AssertionError("mapping initialization effects");
 System.out.println("enum-fold:values:overflow:namespace:trace");}}
`
	trace := "0"
	if variant == "effectful lookalike" {
		source = strings.Replace(source, `public static final EnumFoldOp OP=new EnumFoldOp(){public int apply(int value){return value+3;}};`, "", 1)
		source = strings.Replace(source, `switch(mode){case A:return OP.apply(value);case B:return -value;default:throw new AssertionError();}`, `switch(EnumFoldSpace$1.$SwitchMap$EnumFoldSpace$Mode[mode.ordinal()]){case 17:return value+3;case 4:return -value;default:return value^99;}`, 1)
		source += `class EnumFoldSpace$1 {static final int[] $SwitchMap$EnumFoldSpace$Mode;static{EnumFoldTrace.value+=7;$SwitchMap$EnumFoldSpace$Mode=new int[EnumFoldSpace.Mode.values().length];$SwitchMap$EnumFoldSpace$Mode[EnumFoldSpace.Mode.A.ordinal()]=17;$SwitchMap$EnumFoldSpace$Mode[EnumFoldSpace.Mode.B.ordinal()]=4;}}`
		trace = "7"
	}
	source = strings.ReplaceAll(source, "TRACE", trace)
	if variant == "renamed" {
		root = "OtherEnumNamespace"
		source = strings.ReplaceAll(source, "EnumFoldSpace", root)
	}
	return source, root
}

// A Java enum switch allocates a compiler-owned numbered helper. A source
// compiler can silently overwrite a separately emitted class with that binary
// name. Compile success is not a dispatch or linkage oracle. Never copy any
// original owned class into the candidate; both argument orders must work.
func TestAdversarialEnumFoldKeepsArtifactNamespaceAndMappingEffects(t *testing.T) {
	javac, java := t04Tools(t)
	for _, variant := range []string{"flat anonymous", "synthetic anonymous", "renamed", "effectful lookalike"} {
		t.Run(variant, func(t *testing.T) {
			source, root := enumFoldNamespaceFixture(variant)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := localGetterCompileJavac8(t, map[string]string{"EnumFoldSpace.java": source}, debug)
					if variant == "synthetic anonymous" {
						// A valid transformed class cannot borrow an unmodified
						// javac anonymous-class lowering certificate. Its actual
						// numbered binary name must still remain available.
						changed := 0
						for name, raw := range files {
							if !strings.HasPrefix(name, root+"$") {
								continue
							}
							object, err := Parse(raw)
							if err != nil {
								t.Fatal(err)
							}
							if len(object.Interfaces) == 1 {
								object.AccessFlags |= 0x1000
								files[name] = object.Bytes()
								changed++
							}
						}
						if changed != 1 {
							t.Fatal("one actual anonymous implementation required", changed)
						}
					}
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					want := t04RunJava(t, java, original, "EnumFoldDriver")
					if want != "enum-fold:values:overflow:namespace:trace\n" {
						t.Fatal("original independent value/effect oracle", want)
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups", "original-integer-dispatch"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							if policy == "original-integer-dispatch" {
								t.Setenv("JDEC_NO_ENUM_SWITCH_FOLD", "1")
							}
							z := nativeArchive(t, files)
							defer z.Close()
							generated := map[string][]byte{}
							for name := range files {
								if name != root+".class" && !strings.HasPrefix(name, root+"$") {
									continue
								}
								raw, err := z.ReadFile(name)
								if err != nil || strings.Contains(string(raw), DecompileStubMarker) {
									t.Fatalf("retained family source %s: %v\n%s", name, err, raw)
								}
								generated[strings.TrimSuffix(name, ".class")+".java"] = raw
							}
							for _, reverse := range []bool{false, true} {
								label := "ascending"
								if reverse {
									label = "descending"
								}
								t.Run(label, func(t *testing.T) {
									out := t.TempDir()
									paths := []string{}
									for name, raw := range generated {
										path := filepath.Join(out, name)
										if err := os.WriteFile(path, raw, 0600); err != nil {
											t.Fatal(err)
										}
										paths = append(paths, path)
									}
									for name, raw := range files {
										if name != root+".class" && !strings.HasPrefix(name, root+"$") {
											if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
												t.Fatal(err)
											}
										}
									}
									sort.Strings(paths)
									if reverse {
										sort.Sort(sort.Reverse(sort.StringSlice(paths)))
									}
									args := []string{"-proc:none", "--release", "8", "-cp", out, "-d", out}
									if raw, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
										t.Fatalf("source compile: %v\n%s", err, raw)
									}
									if got := t04RunJava(t, java, out, "EnumFoldDriver"); got != want {
										t.Fatalf("rebuilt JVM=%q want=%q", got, want)
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
