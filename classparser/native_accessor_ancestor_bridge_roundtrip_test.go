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

// An erased interface bridge and a compiler access method can have identical
// erased signatures. javac deliberately excludes bridges from synthetic-name
// conflicts. Rebuild the ancestor too: leaving it binary would conceal the
// mistaken assumption that a flat ancestor emits every bridge as ordinary.
func TestNativeAccessorAncestorBridgePreservesErasedInterfaceDispatch(t *testing.T) {
	javac, _ := t04Tools(t)
	testNativeAccessorAncestorBridgeDispatch(t, javac, []string{"--release", "8"}, ModernJavac, []string{"Flag", "Harbor"}, []string{"none", "source,lines,vars"}, func(t *testing.T, sources map[string]string, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	})
}
func TestNativeAccessorAncestorBridgeOriginalJavac8Dispatch(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent original and rebuild compiler")
	}
	testNativeAccessorAncestorBridgeDispatch(t, javac, []string{"-source", "8", "-target", "8"}, NativeJavac8, []string{"Flag"}, []string{"none"}, fieldOwnerCompileJava8Sources)
}
func testNativeAccessorAncestorBridgeDispatch(t *testing.T, javac string, compilerArgs []string, profile SourceCompilerProfile, prefixes, debugModes []string, compile func(*testing.T, map[string]string, string) map[string][]byte) {
	_, java := t04Tools(t)
	for _, operation := range []string{"return", "return and argument"} {
		for _, prefix := range prefixes {
			for _, debug := range debugModes {
				t.Run(operation+"/"+prefix+"/"+debug, func(t *testing.T) {
					iface := `interface FlagValue<T>{T access$000();}`
					parent := `class FlagParent extends p.FlagBase implements FlagValue<String>{public String access$000(){calls++;return "bridge";}}`
					reader := `public Object read(){return FlagRoot.value;}`
					check := `if(new FlagRoot.Reader().read()!=v||!((FlagValue<?>)root).access$000().equals("bridge")||FlagParent.calls!=before+1)throw new AssertionError("bridge vs static getter binding");`
					bridges := "1"
					if operation == "return and argument" {
						iface = `interface FlagValue<T>{T access$000();T convert(T value);}`
						parent = `class FlagParent extends p.FlagBase implements FlagValue<String>{public String access$000(){calls++;return "bridge";}public String convert(String v){calls++;return v;}}`
						check += `String s=new String("bridge");if(((FlagValue<String>)root).convert(s)!=s||FlagParent.calls!=before+2)throw new AssertionError("erased interface dispatch/identity");try{((FlagValue)root).convert(new Object());throw new AssertionError("missing bridge CHECKCAST");}catch(ClassCastException expected){if(FlagParent.calls!=before+2||p.FlagBase.get()!=v)throw new AssertionError("bridge CHECKCAST must precede typed body");}if(((FlagValue<String>)root).convert(null)!=null||FlagParent.calls!=before+3)throw new AssertionError("null bridge argument");`
						bridges = "2"
					}
					parent = strings.Replace(parent, "{public String", "{static int calls;public String", 1)
					program := `package q;` + iface + parent + `class FlagRoot extends FlagParent{public static class Reader{` + reader + `}}class FlagDriver{public static void main(String[]args)throws Exception{
String[]cp=System.getProperty("java.class.path").split(java.io.File.pathSeparator);java.io.File target=new java.io.File(cp[0]).getCanonicalFile(),original=new java.io.File(cp[cp.length-1]).getCanonicalFile();boolean source=args.length==0||args[0].equals("source");for(Class<?> c:new Class<?>[]{FlagRoot.class,FlagRoot.Reader.class,FlagParent.class,FlagValue.class,p.FlagBase.class,FlagDriver.class}){java.io.File expected=c==FlagRoot.class||c==FlagRoot.Reader.class||source&&(c==FlagParent.class||c==FlagValue.class)?target:original;if(!new java.io.File(c.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(expected))throw new AssertionError("wrong source/binary origin "+c);}
if(FlagRoot.Reader.class.getDeclaringClass()!=FlagRoot.class)throw new AssertionError("source scope lost");int bridges=0;for(java.lang.reflect.Method m:FlagParent.class.getDeclaredMethods())if(m.isBridge()&&m.isSynthetic())bridges++;java.lang.reflect.Method access=FlagRoot.class.getDeclaredMethod("access$000");if(bridges!=` + bridges + `||!access.isSynthetic()||!java.lang.reflect.Modifier.isStatic(access.getModifiers()))throw new AssertionError("original bridge/private-accessor flags");FlagRoot root=new FlagRoot();for(Object v:new Object[]{null,new Object()}){p.FlagBase.set(v);int before=FlagParent.calls;` + check + `}System.out.println("bridge:accessor:original:binding:scope");}}`
					sources := map[string]string{"p/" + prefix + "Base.java": strings.ReplaceAll(`package p;public class FlagBase{protected static Object value=new Object();public static void set(Object v){value=v;}public static Object get(){return value;}}`, "Flag", prefix), "q/" + prefix + "Program.java": strings.ReplaceAll(program, "Flag", prefix)}
					files := compile(t, sources, debug)
					write := func(dir, n string, raw []byte) {
						t.Helper()
						p := filepath.Join(dir, n)
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					original := t.TempDir()
					for n, raw := range files {
						write(original, n, raw)
					}
					run := func(t *testing.T, cp, world string) {
						t.Helper()
						out, e := exec.Command(java, "-Xverify:all", "-cp", cp, "q."+prefix+"Driver", world).CombinedOutput()
						if e != nil || string(out) != "bridge:accessor:original:binding:scope\n" {
							t.Fatalf("original-driver %s:%v\n%s", world, e, out)
						}
					}
					run(t, original, "source")
					for _, world := range []string{"source", "binary"} {
						t.Run(world, func(t *testing.T) {
							input := map[string][]byte{}
							for n, raw := range files {
								if world == "source" || n == "q/"+prefix+"Root.class" || strings.HasPrefix(n, "q/"+prefix+"Root$") {
									input[n] = raw
								}
							}
							jar := filepath.Join(t.TempDir(), "original.jar")
							write(filepath.Dir(jar), filepath.Base(jar), t23Zip(t, input))
							resolve := func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k }
							z, e := NewJarFSFromLocalWithCompilerProfile(jar, 8, profile, resolve)
							if e != nil {
								t.Fatal(e)
							}
							defer z.Close()
							out := t.TempDir()
							var paths []string
							var names []string
							for n := range input {
								names = append(names, n)
							}
							sort.Strings(names)
							for _, n := range names {
								if n == "p/"+prefix+"Base.class" || n == "q/"+prefix+"Driver.class" {
									continue
								}
								source, e := z.ReadFile(n)
								if e != nil || strings.Contains(string(source), DecompileStubMarker) {
									t.Fatalf("source %s:%v\n%s", n, e, source)
								}

								path := strings.TrimSuffix(n, ".class") + ".java"
								write(out, path, source)
								paths = append(paths, filepath.Join(out, path))
							}
							if log, e := exec.Command(javac, append(append(append([]string{"-proc:none"}, compilerArgs...), "-cp", original, "-d", out), paths...)...).CombinedOutput(); e != nil {
								t.Fatalf("joint bridge source compilation:%v\n%s", e, log)
							}
							for n, raw := range files {
								retained, e := os.ReadFile(filepath.Join(original, n))
								if e != nil || !bytes.Equal(retained, raw) {
									t.Fatal("original producer/driver replaced", n)
								}
							}
							run(t, out+string(os.PathListSeparator)+original, world)
						})
					}
				})
			}
		}
	}
}
