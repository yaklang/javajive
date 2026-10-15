package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The original joint compilation has synthetic access$000 in both subclasses.
// A constructor-local declaration makes only the intermediate family refuse
// lexical reconstruction. Its independently flat source retains that method
// as ordinary Java. Regenerating the leaf's synthetic method then conflicts
// with the ordinary inherited method, despite both original packets being valid.
// The driver and all producers stay original; both source families are rebuilt
// together, so a binary parent cannot hide the source-only conflict.
func TestAdversarialInheritedAccessorKeepsJointSourceAndInitialization(t *testing.T) {
	javac, java := t04Tools(t)
	for _, names := range [][2]string{{"Parent", "Root"}, {"Intermediate", "Leaf"}} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(names[0]+"/"+debug, func(t *testing.T) {
				ss := map[string]string{
					"p/Effects.java": `package p;public class Effects{public static String trace="";public static final RuntimeException failure=new RuntimeException("original");public static boolean fail;}`,
					"p/Base.java":    `package p;public class Base{protected static Object value=init();static Object init(){Effects.trace+="B";return new Object();}public static void set(Object v){value=v;}}`,
					"q/Parent.java":  `package q;public class Parent extends p.Base{static{p.Effects.trace+="P";}public Parent(){class Local{};new Local();}public static class BuilderBase{public Object read(){return Parent.value;}}}`,
					"q/Root.java":    `package q;public class Root extends Parent{static{p.Effects.trace+="R";if(p.Effects.fail)throw p.Effects.failure;}public static class Builder extends Parent.BuilderBase{public Object read(){return Root.value;}}}`,
					"q/Driver.java":  `package q;public class Driver{public static void main(String[]args)throws Exception{p.Effects.trace="";Root.Builder reader=new Root.Builder();if(args.length>0){p.Effects.fail=true;try{reader.read();throw new AssertionError("missing init failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=p.Effects.failure||!p.Effects.trace.equals("BPR"))throw new AssertionError("cause/order",e);}p.Effects.trace="";try{reader.read();throw new AssertionError("missing erroneous class state");}catch(NoClassDefFoundError e){if(!p.Effects.trace.equals(""))throw new AssertionError("retried initialization");}System.out.println("original:init:failure:identity");return;}Object token=reader.read();if(token==null||!p.Effects.trace.equals("BPR")||new Parent.BuilderBase().read()!=token)throw new AssertionError("declaring initialization/identity:"+p.Effects.trace);if(Root.class.getDeclaredMethod("access$000").getDeclaringClass()!=Root.class)throw new AssertionError("changed accessor declaring class");for(Object v:new Object[]{null,new Object()}){p.Base.set(v);if(reader.read()!=v||new Parent.BuilderBase().read()!=v)throw new AssertionError("shared original field");}System.out.println("original:init:order:mutable:identity");}}`,
				}
				sources := map[string]string{}
				rename := func(s string) string {
					return strings.ReplaceAll(strings.ReplaceAll(s, "Parent", names[0]), "Root", names[1])
				}
				for n, s := range ss {
					sources[rename(n)] = rename(s)
				}
				files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
				write := func(dir, name string, raw []byte) {
					t.Helper()
					path := filepath.Join(dir, name)
					if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
						t.Fatal(e)
					}
					if e := os.WriteFile(path, raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
				original := t.TempDir()
				for n, b := range files {
					write(original, n, b)
				}
				run := func(dir, mode, want string) {
					t.Helper()
					args := []string{"-Xverify:all", "-cp", dir, "q.Driver"}
					if mode != "" {
						args = append(args, mode)
					}
					got, e := exec.Command(java, args...).CombinedOutput()
					if e != nil || string(got) != want {
						t.Fatalf("original-driver %s: %v\n%s want=%s", mode, e, got, want)
					}
				}
				const success = "original:init:order:mutable:identity\n"
				const failure = "original:init:failure:identity\n"
				run(original, "", success)
				run(original, "failure", failure)
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(policy, func(t *testing.T) {
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeArchive(t, files)
						defer z.Close()
						parent, _ := Parse(files["q/"+names[0]+".class"])
						entry := z.nativeMemberEntry(parent)
						if entry == nil || entry.planKnown || entry.family != nil {
							t.Fatal("fixture must exercise a terminally flat intermediate source family")
						}
						out := t.TempDir()
						var paths []string
						for n, raw := range files {
							// The unsupported constructor-local producer stays original.
							// Every named class participating in the source collision is rebuilt.
							if !strings.HasPrefix(n, "q/"+names[0]) && !strings.HasPrefix(n, "q/"+names[1]) || strings.HasSuffix(n, "$1Local.class") {
								write(out, n, raw)
								continue
							}
							source, e := z.ReadFile(n)
							if e != nil || strings.Contains(string(source), DecompileStubMarker) {
								t.Fatalf("source %s: %v\n%s", n, e, source)
							}
							path := strings.TrimSuffix(n, ".class") + ".java"
							write(out, path, source)
							paths = append(paths, filepath.Join(out, path))
						}
						if log, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); e != nil {
							t.Fatalf("joint source compilation: %v\n%s", e, log)
						}
						run(out, "", success)
						run(out, "failure", failure)
					})
				}
			})
		}
	}
}
