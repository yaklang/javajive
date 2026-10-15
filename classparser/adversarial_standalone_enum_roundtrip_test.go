package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Only the requested class is rebuilt. The driver, enum and effect producers
// remain original bytecode: a matching mistake in a regenerated oracle cannot
// hide a change in dispatch, null behavior, overflow or selector evaluation.
func TestAdversarialStandaloneEnumOriginalDriverRoundTrip(t *testing.T) {
	const fixture = `enum SingleMode { A,B,C }
class SingleEffects {static int trace;static SingleMode select(SingleMode mode){trace=trace*7+3;return mode;}}
class SingleRoot {
 static int direct(SingleMode mode,int value){switch(mode){case A:return value+7;case B:return value^19;default:return -value;}}
 static int computed(SingleMode mode,int value){switch(SingleEffects.select(mode)){case A:return value-5;case B:return value*31;default:return value>>>1;}}
}
class SingleDriver {public static void main(String[]args){int rows=0;
 for(int repeat=0;repeat<3;repeat++)for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(SingleMode mode:SingleMode.values()){
  int direct=mode==SingleMode.A?value+7:mode==SingleMode.B?value^19:-value;
  int computed=mode==SingleMode.A?value-5:mode==SingleMode.B?value*31:value>>>1;
  SingleEffects.trace=0;
  if(SingleRoot.direct(mode,value)!=direct||SingleEffects.trace!=0)throw new AssertionError("dispatch/overflow/direct effects");
  if(SingleRoot.computed(mode,value)!=computed||SingleEffects.trace!=3)throw new AssertionError("selector evaluated exactly once");rows++;
 }
 SingleEffects.trace=0;
 try{SingleRoot.direct(null,1);throw new AssertionError("null direct");}catch(NullPointerException expected){}
 if(SingleEffects.trace!=0)throw new AssertionError("direct null effects");
 try{SingleRoot.computed(null,1);throw new AssertionError("null computed");}catch(NullPointerException expected){}
 if(SingleEffects.trace!=3)throw new AssertionError("computed null effects");
 System.out.println(rows+":standalone:dispatch:null:overflow:effects");}}
`
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Single", "Renamed"} {
		t.Run(prefix, func(t *testing.T) {
			for _, compiler := range []string{"modern", "javac8"} {
				t.Run(compiler, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							source := strings.ReplaceAll(fixture, "Single", prefix)
							var files map[string][]byte
							if compiler == "javac8" {
								files = localGetterCompileJavac8(t, map[string]string{prefix + "Root.java": source}, debug)
							} else {
								files = nativeCompileDebugClasses(t, source, debug)
							}
							original := t.TempDir()
							for name, raw := range files {
								if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							want := t04RunJava(t, java, original, prefix+"Driver")
							if want != "45:standalone:dispatch:null:overflow:effects\n" {
								t.Fatal("original independent oracle", want)
							}
							resolver := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
								t.Run(string(mode), func(t *testing.T) {
									raw := files[prefix+"Root.class"]
									var generated string
									if mode == "legacy" {
										var err error
										generated, err = DecompileWithResolver(raw, resolver)
										if err != nil {
											t.Fatal(err)
										}
									} else {
										result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolver, TargetSourceVersion: 8})
										if err != nil {
											t.Fatal(err)
										}
										generated = result.Source
									}
									if strings.Contains(generated, DecompileStubMarker) {
										t.Fatal("stub in candidate", generated)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name == prefix+"Root.class" || strings.HasPrefix(name, prefix+"Root$") {
											continue
										}
										if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
											t.Fatal(err)
										}
									}
									path := filepath.Join(out, prefix+"Root.java")
									if err := os.WriteFile(path, []byte(generated), 0600); err != nil {
										t.Fatal(err)
									}
									if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); err != nil {
										t.Fatalf("rebuild requested source without original root/helper fallback: %v\n%s\n%s", err, raw, generated)
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
}
