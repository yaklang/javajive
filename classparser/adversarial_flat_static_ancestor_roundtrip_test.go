package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A flat static member can be an ancestor directly or through another class.
// Its ordinary accessor can collide with a regenerated descendant accessor even
// when the physical outer has no matching method and the ancestor never occurs
// in the descendant's constant pool. Rebuild both source families together;
// binary ancestors would conceal this source-only flag conflict. Original
// drivers, field producers and constructor-local producer bytes are retained.
func TestAdversarialFlatStaticAncestorKeepsOriginalAccessorsAndEffects(t *testing.T) {
	javac, java := t04Tools(t)
	for _, route := range []string{"direct", "through intermediate"} {
		for _, names := range [][2]string{{"Parent", "Root"}, {"Intermediate", "Leaf"}} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(route+"/"+names[0]+"/"+debug, func(t *testing.T) {
					ss := map[string]string{
						"p/Effects.java": `package p;public class Effects{public static String trace="";public static final RuntimeException failure=new RuntimeException("original");public static boolean fail;}`,
						"p/Base.java":    `package p;public class Base{protected static Object value=init();static Object init(){Effects.trace+="B";return new Object();}public static void set(Object v){value=v;}}`,
						"q/Holder.java":  `package q;public class Holder{public static class Parent extends p.Base{static{p.Effects.trace+="P";}public Parent(){class Local{};new Local();}public static class BuilderBase{public Object read(){return Parent.value;}}}}`,
						"q/Root.java":    `package q;public class Root extends Holder.Parent{static{p.Effects.trace+="R";if(p.Effects.fail)throw p.Effects.failure;}public static class Builder{public Object read(){return Root.value;}}}`,
						"q/Driver.java":  `package q;public class Driver{public static void main(String[]args)throws Exception{p.Effects.trace="";Root.Builder reader=new Root.Builder();if(args.length>0){p.Effects.fail=true;try{reader.read();throw new AssertionError("missing init failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=p.Effects.failure||!p.Effects.trace.equals("BPR"))throw new AssertionError("cause/order",e);}p.Effects.trace="";try{reader.read();throw new AssertionError("missing erroneous class state");}catch(NoClassDefFoundError e){if(!p.Effects.trace.equals(""))throw new AssertionError("retried initialization");}System.out.println("original:init:failure:identity");return;}Object token=reader.read();if(token==null||!p.Effects.trace.equals("BPR")||new Holder.Parent.BuilderBase().read()!=token)throw new AssertionError("declaring initialization/identity:"+p.Effects.trace);if(Root.class.getDeclaredMethod("access$000").getDeclaringClass()!=Root.class)throw new AssertionError("changed accessor declaring class");for(Object v:new Object[]{null,new Object()}){p.Base.set(v);if(reader.read()!=v||new Holder.Parent.BuilderBase().read()!=v)throw new AssertionError("shared original field");}System.out.println("original:init:order:mutable:identity");}}`,
					}
					if route == "through intermediate" {
						ss["q/Middle.java"] = `package q;public class Middle extends Holder.Parent{public Middle(){super();}}`
						ss["q/Root.java"] = strings.Replace(ss["q/Root.java"], "extends Holder.Parent{", "extends Middle{", 1)
					}
					sources := map[string]string{}
					rename := func(s string) string {
						return strings.ReplaceAll(strings.ReplaceAll(s, "Parent", names[0]), "Root", names[1])
					}
					for n, s := range ss {
						sources[rename(n)] = rename(s)
					}
					files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
					nativeTestForeignSourceProtocolUnproved(t, files, "q/Holder")
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
							parent, _ := Parse(files["q/Holder.class"])
							entry := z.nativeMemberEntry(parent)
							if entry == nil || entry.planKnown || entry.family != nil {
								t.Fatal("fixture must exercise a terminally flat intermediate source family")
							}
							out := t.TempDir()
							var paths []string
							for n, raw := range files {
								// The unsupported constructor-local producer stays original.
								// Every named class participating in the source collision is rebuilt.
								if !strings.HasPrefix(n, "q/Holder") && !strings.HasPrefix(n, "q/"+names[1]) || strings.HasSuffix(n, "$1Local.class") {
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
							for n, originalBytes := range files {
								if strings.HasPrefix(n, "q/Holder") || strings.HasPrefix(n, "q/"+names[1]) {
									if !strings.HasSuffix(n, "$1Local.class") {
										continue
									}
								}
								retained, e := os.ReadFile(filepath.Join(out, n))
								if e != nil || !bytes.Equal(retained, originalBytes) {
									t.Fatalf("original producer/driver replaced: %s %v", n, e)
								}
							}
							run(out, "", success)
							run(out, "failure", failure)
						})
					}
				})
			}
		}
	}
}
