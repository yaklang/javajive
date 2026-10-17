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
func TestAdversarialInheritedSetterErasedBridgeDispatch(t *testing.T) {
	javac, java := t04Tools(t)
	for _, operation := range []string{"return", "parameter"} {
		for _, prefix := range []string{"Flag", "Harbor"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(operation+"/"+prefix+"/"+debug, func(t *testing.T) {
					iface := `interface FlagValue<T>{T access$000();}`
					parent := `class FlagParent extends p.FlagBase implements FlagValue<String>{public String access$000(){calls++;return "bridge";}}`
					reader := `public Object read(){return FlagRoot.value;}`
					check := `if(new FlagRoot.Reader().read()!=v||!((FlagValue<?>)root).access$000().equals("bridge")||FlagParent.calls!=before+1)throw new AssertionError("bridge vs static getter binding");`
					method, params := "access$000", ""
					if operation == "parameter" {
						iface = `interface FlagValue<T>{T access$002(T value);}`
						parent = `class FlagParent extends p.FlagBase implements FlagValue<String>{public String access$002(String v){calls++;return v;}}`
						reader = `public Object write(Object v){return FlagRoot.value=v;}`
						check = `if(new FlagRoot.Reader().write(v)!=v||p.FlagBase.get()!=v||FlagParent.calls!=before)throw new AssertionError("static setter effect/identity");String s=new String("bridge");if(((FlagValue<String>)root).access$002(s)!=s||FlagParent.calls!=before+1)throw new AssertionError("erased interface dispatch/identity");try{((FlagValue)root).access$002(new Object());throw new AssertionError("missing bridge CHECKCAST");}catch(ClassCastException expected){if(FlagParent.calls!=before+1||p.FlagBase.get()!=v)throw new AssertionError("bridge CHECKCAST must precede typed body");}if(((FlagValue<String>)root).access$002(null)!=null||FlagParent.calls!=before+2)throw new AssertionError("null bridge argument");`
						method, params = "access$002", "Object.class"
					}
					parent = strings.Replace(parent, "{public String", "{static int calls;public String", 1)
					program := `package q;` + iface + parent + `class FlagRoot extends FlagParent{public static class Reader{` + reader + `}}class FlagDriver{public static void main(String[]args)throws Exception{
String[]cp=System.getProperty("java.class.path").split(java.io.File.pathSeparator);java.io.File target=new java.io.File(cp[0]).getCanonicalFile(),original=new java.io.File(cp[cp.length-1]).getCanonicalFile();boolean source=args.length==0||args[0].equals("source");for(Class<?> c:new Class<?>[]{FlagRoot.class,FlagRoot.Reader.class,FlagParent.class,FlagValue.class,p.FlagBase.class,FlagDriver.class}){java.io.File expected=c==FlagRoot.class||c==FlagRoot.Reader.class||source&&(c==FlagParent.class||c==FlagValue.class)?target:original;if(!new java.io.File(c.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(expected))throw new AssertionError("wrong source/binary origin "+c);}
if(FlagRoot.Reader.class.getDeclaringClass()!=FlagRoot.class)throw new AssertionError("source scope lost");int bridges=0;for(java.lang.reflect.Method m:FlagParent.class.getDeclaredMethods())if(m.isBridge()&&m.isSynthetic())bridges++;java.lang.reflect.Method access=FlagRoot.class.getDeclaredMethod("` + method + `"` + func() string {
						if params != "" {
							return "," + params
						}
						return ""
					}() + `);if(bridges!=1||!access.isSynthetic()||!java.lang.reflect.Modifier.isStatic(access.getModifiers()))throw new AssertionError("original bridge/private-accessor flags");FlagRoot root=new FlagRoot();for(Object v:new Object[]{null,new Object()}){p.FlagBase.set(v);int before=FlagParent.calls;` + check + `}System.out.println("bridge:accessor:original:binding:scope");}}`
					sources := map[string]string{"p/" + prefix + "Base.java": strings.ReplaceAll(`package p;public class FlagBase{protected static Object value=new Object();public static void set(Object v){value=v;}public static Object get(){return value;}}`, "Flag", prefix), "q/" + prefix + "Program.java": strings.ReplaceAll(program, "Flag", prefix)}
					files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
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
							z, e := NewJarFSFromLocalWithSourceVersion(jar, 8, resolve)
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
							if log, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", original, "-d", out}, paths...)...).CombinedOutput(); e != nil {
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
