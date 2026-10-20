package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
)

// The outer protected region also owns failures raised by the inner handler.
// Reflection syntax in that handler is not evidence for introducing a wrapper.
func TestAdversarialNestedReflectiveFallbackPreservesOriginalExceptionObject(t *testing.T) {
	testNestedReflectiveFallback(t, reflectiveFallbackFixture)
}

// Exception remains source-reachable even after a bogus inner wrapper. This
// variant therefore kills an executable semantic mutant, not just a javac error.
func TestAdversarialNestedReflectiveFallbackKeepsPayloadUnderBroadOuterCatch(t *testing.T) {
	source := strings.ReplaceAll(reflectiveFallbackFixture, "catch(SecurityException failure){return failure;}catch(ReflectiveOperationException|RuntimeException failure){return failure;}", "catch(Exception failure){return failure;}")
	if source == reflectiveFallbackFixture {
		t.Fatal("original broad-handler variant not constructed")
	}
	testNestedReflectiveFallback(t, source)
}

func testNestedReflectiveFallback(t *testing.T, fixture string) {
	t.Helper()
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "ReflectiveFallbackDriver", debug, false)
					write := func(dir, name string, raw []byte) {
						t.Helper()
						if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					original := t.TempDir()
					for name, raw := range files {
						write(original, name, raw)
					}
					want := t04RunJava(t, java, original, "ReflectiveFallbackDriver")
					if want != "150:reflection:fallback:original-identity:ordered-domains\n" {
						t.Fatal("original independent model", want)
					}
					for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
						t.Run(string(mode), func(t *testing.T) {
							input := bytes.Clone(files["ReflectiveFallbackOwner.class"])
							resolve := func(name string) ([]byte, bool) { raw, found := files[name+".class"]; return bytes.Clone(raw), found }
							var result DecompileResult
							err := jdecenv.Run(map[string]string{}, func() error {
								var e error
								if mode == "legacy" {
									result.Source, e = DecompileWithResolver(input, resolve)
								} else {
									result, e = DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: resolve})
								}
								return e
							})
							if err != nil || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) || !bytes.Equal(input, files["ReflectiveFallbackOwner.class"]) {
								t.Fatalf("source mode=%s status=%s err=%v diagnostics=%+v\n%s", mode, result.Status, err, result.Diagnostics, result.Source)
							}
							out := t.TempDir()
							for name, raw := range files {
								if name != "ReflectiveFallbackOwner.class" {
									write(out, name, raw)
								}
							}
							write(out, "ReflectiveFallbackOwner.java", []byte(result.Source))
							args := []string{"-proc:none", "--release", "8"}
							if profile == NativeJavac8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							args = append(args, "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, filepath.Join(out, "ReflectiveFallbackOwner.java"))
							if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
								t.Fatalf("recompile with original target masked: %v\n%s\n%s", err, log, result.Source)
							}
							if got := t04RunJava(t, java, out, "ReflectiveFallbackDriver"); got != want {
								t.Fatalf("unchanged binary caller %q want %q", got, want)
							}
							candidate, err := os.ReadFile(filepath.Join(out, "ReflectiveFallbackOwner.class"))
							if err != nil || constructorInvocationThrowsDeclarations(t, input) != constructorInvocationThrowsDeclarations(t, candidate) {
								t.Fatal("original member/throws inventory changed", err)
							}
						})
					}
				})
			}
		})
	}
}

const reflectiveFallbackFixture = `class ReflectiveFallbackOps{
 static String trace="";static String name(String value,String phase){trace+=phase;return value;}
}
class ReflectiveFallbackProbe{public void present(){}}
class ReflectiveFallbackLoader extends ClassLoader{
 final ClassNotFoundException checked=new ClassNotFoundException("same checked");
 final RuntimeException runtime=new IllegalStateException("same runtime");
 final SecurityException security=new SecurityException("same security");
 final Error fatal=new AssertionError("same fatal");
 public Class<?> loadClass(String name)throws ClassNotFoundException{
  if(name.equals("missing.Checked"))throw checked;if(name.equals("missing.Runtime"))throw runtime;
  if(name.equals("missing.Security"))throw security;if(name.equals("missing.Fatal"))throw fatal;
  return super.loadClass(name);
 }
}
class ReflectiveFallbackOwner{
 static Object byName(String first,String second,String method,ClassLoader loader){
  try{try{return Class.forName(ReflectiveFallbackOps.name(first,"A"),true,loader);}
   catch(SecurityException failure){throw failure;}
   catch(ReflectiveOperationException|RuntimeException failure){return Class.forName(ReflectiveFallbackOps.name(second,"B"),true,loader);}}
  catch(SecurityException failure){return failure;}catch(ReflectiveOperationException|RuntimeException failure){return failure;}
 }
 static Object publicMethod(String first,String second,String method,ClassLoader loader){
  try{try{return Class.forName(ReflectiveFallbackOps.name(first,"A"),true,loader).getMethod(method,new Class[0]);}
   catch(SecurityException failure){throw failure;}
   catch(ReflectiveOperationException|RuntimeException failure){return Class.forName(ReflectiveFallbackOps.name(second,"B"),true,loader).getMethod(method,new Class[0]);}}
  catch(SecurityException failure){return failure;}catch(ReflectiveOperationException|RuntimeException failure){return failure;}
 }
 static Object declaredMethod(String first,String second,String method,ClassLoader loader){
  try{try{return Class.forName(ReflectiveFallbackOps.name(first,"A"),true,loader).getDeclaredMethod(method,new Class[0]);}
   catch(SecurityException failure){throw failure;}
   catch(ReflectiveOperationException|RuntimeException failure){return Class.forName(ReflectiveFallbackOps.name(second,"B"),true,loader).getDeclaredMethod(method,new Class[0]);}}
  catch(SecurityException failure){return failure;}catch(ReflectiveOperationException|RuntimeException failure){return failure;}
 }
}
public class ReflectiveFallbackDriver{public static void main(String[]args){int worlds=0;
 String[] names={"ReflectiveFallbackProbe","missing.Checked","missing.Runtime","missing.Security","missing.Fatal"};
 for(int kind=0;kind<3;kind++)for(int first=0;first<5;first++)for(int second=0;second<5;second++)for(boolean present:new boolean[]{false,true}){
  ReflectiveFallbackLoader loader=new ReflectiveFallbackLoader();ReflectiveFallbackOps.trace="";String method=present?"present":"absent";Object result;boolean escaped=false;
  try{result=kind==0?ReflectiveFallbackOwner.byName(names[first],names[second],method,loader):kind==1?ReflectiveFallbackOwner.publicMethod(names[first],names[second],method,loader):ReflectiveFallbackOwner.declaredMethod(names[first],names[second],method,loader);}catch(Throwable failure){result=failure;escaped=true;}
  boolean fallback=first==1||first==2||first==0&&kind!=0&&!present;int outcome=fallback?second:first;
  if(!ReflectiveFallbackOps.trace.equals(fallback?"AB":"A")||escaped!=(outcome==4))throw new AssertionError("protected order/escape: "+ReflectiveFallbackOps.trace);
  if(outcome==1&&result!=loader.checked||outcome==2&&result!=loader.runtime||outcome==3&&result!=loader.security||outcome==4&&result!=loader.fatal)throw new AssertionError("original exception object replaced",result instanceof Throwable?(Throwable)result:null);
  if(outcome==0){if(kind==0){if(result!=ReflectiveFallbackProbe.class)throw new AssertionError("class identity");}else if(present){if(!(result instanceof java.lang.reflect.Method)||!((java.lang.reflect.Method)result).getName().equals("present"))throw new AssertionError("selected method");}else if(!(result instanceof NoSuchMethodException))throw new AssertionError("missing method category");}
  worlds++;
 }System.out.println(worlds+":reflection:fallback:original-identity:ordered-domains");}}
`
