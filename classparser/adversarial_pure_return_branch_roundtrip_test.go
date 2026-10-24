package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialPureReturnBranchesPreserveNormalArm(t *testing.T) {
	const fixture = `class BindingEffects{static int trace;static final RuntimeException failed=new IllegalArgumentException("parse");static Object parse(int mode,Object value){trace=trace*7+1;if(mode==3)throw failed;return mode==0?value:null;}static final RuntimeException after=new IllegalStateException("after");static Object fallback(int mode){trace=trace*7+2;if(mode==4)throw after;return mode==2?"fallback":null;}static Object handle(Exception fail){trace=trace*7+3;if(fail!=failed)throw new AssertionError("exception identity");return "handled";}}
class BindingAccess{
 static Object shared(int mode,Object input){if(input==null)return null;try{Object result=BindingEffects.parse(mode,input);if(result!=null)return result;}catch(Exception failure){return BindingEffects.handle(failure);}if(mode==1)return null;return BindingEffects.fallback(mode);}
 static Object nested(int mode,Object input){try{Object result;try{result=BindingEffects.parse(mode,input);}catch(IllegalArgumentException failure){return BindingEffects.handle(failure);}if(result==null)return null;return result;}catch(Exception failure){return BindingEffects.handle(failure);}}
}
class BindingDriver{public static void main(String[]args){int rows=0;Object identity=new Object();for(int repeat=0;repeat<3;repeat++)for(int shape=0;shape<2;shape++)for(int mode=0;mode<5;mode++)for(Object input:new Object[]{identity,null}){BindingEffects.trace=0;Object got=null;Throwable failure=null;try{got=shape==0?BindingAccess.shared(mode,input):BindingAccess.nested(mode,input);}catch(Throwable caught){failure=caught;}Object want;int trace;if(shape==0&&input==null){want=null;trace=0;}else if(mode==3){want="handled";trace=10;}else if(shape==1){want=mode==0?input:null;trace=1;}else if(mode==0){want=input;trace=1;}else if(mode==1){want=null;trace=1;}else{want=mode==2?"fallback":null;trace=9;}Throwable expectedFailure=shape==0&&input!=null&&mode==4?BindingEffects.after:null;if(failure!=expectedFailure||got!=want||BindingEffects.trace!=trace)throw new AssertionError("normal return arm/value/effects "+shape+":"+mode+":"+BindingEffects.trace);rows++;}System.out.println(rows+":pure-return:arm:identity:effects");}}
`
	pureReturnOriginalDriverRoundTrip(t, fixture, "60:pure-return:arm:identity:effects\n")
}
func pureReturnOriginalDriverRoundTrip(t *testing.T, fixture, expected string) {
	t.Helper()
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Binding", "Renamed"} {
		t.Run(prefix, func(t *testing.T) {
			for _, compiler := range []string{"modern", "javac8"} {
				t.Run(compiler, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							source := strings.ReplaceAll(fixture, "Binding", prefix)
							var files map[string][]byte
							if compiler == "javac8" {
								files = localGetterCompileJavac8(t, map[string]string{prefix + "Access.java": source}, debug)
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
							if want != expected {
								t.Fatal("original independent oracle", want)
							}
							resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy", "archive"} {
								t.Run(string(mode), func(t *testing.T) {
									var generated string
									if mode == "archive" {
										z := nativeArchive(t, files)
										defer z.Close()
										z.targetSourceVersion = 8
										raw, err := z.ReadFile(prefix + "Access.class")
										if err != nil {
											t.Fatal(err)
										}
										generated = string(raw)
									} else if mode == "legacy" {
										var err error
										generated, err = DecompileWithResolver(files[prefix+"Access.class"], resolve)
										if err != nil {
											t.Fatal(err)
										}
									} else {
										result, err := DecompileWithOptions(files[prefix+"Access.class"], DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
										if err != nil {
											t.Fatal(err)
										}
										generated = result.Source
									}
									if strings.Contains(generated, DecompileStubMarker) {
										t.Fatal("stub", generated)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name == prefix+"Access.class" {
											continue
										}
										if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
											t.Fatal(err)
										}
									}
									path := filepath.Join(out, prefix+"Access.java")
									if err := os.WriteFile(path, []byte(generated), 0600); err != nil {
										t.Fatal(err)
									}
									if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); err != nil {
										t.Fatalf("rebuild accessor without original fallback: %v\n%s\n%s", err, raw, generated)
									}
									if got := t04RunJava(t, java, out, prefix+"Driver"); got != want {
										t.Fatalf("candidate=%q want=%q\n%s", got, want, generated)
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
