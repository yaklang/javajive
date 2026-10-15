package javaclassparser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const accessorAncestorCycleSource = `package q;
class LeaseParent extends p.LeaseBase{public static class BuilderBase{public Object read(){return LeaseParent.value;}}public static class Link{public LeaseA.Marker peer;}}
class LeaseMiddle extends LeaseParent{}
class LeaseA extends LeaseMiddle{public static class Marker{}public static class Builder{public Object read(){return LeaseA.value;}}}
class LeaseDriver{public static void main(String[]args)throws Exception{String[]cp=System.getProperty("java.class.path").split(java.io.File.pathSeparator);java.io.File target=new java.io.File(cp[0]).getCanonicalFile(),original=new java.io.File(cp[cp.length-1]).getCanonicalFile();boolean source=args.length>0&&args[0].equals("source");for(Class<?> c:new Class<?>[]{LeaseA.class,LeaseParent.class,LeaseMiddle.class,p.LeaseBase.class,LeaseDriver.class}){java.io.File expected=c==LeaseA.class||source&&(c==LeaseParent.class||c==LeaseMiddle.class)?target:original;if(!new java.io.File(c.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(expected))throw new AssertionError("wrong binary/source origin "+c);}
if(LeaseA.Marker.class.getDeclaringClass()!=LeaseA.class||LeaseA.Builder.class.getDeclaringClass()!=LeaseA.class||LeaseParent.Link.class.getDeclaringClass()!=LeaseParent.class||!LeaseA.class.getDeclaredMethod("access$000").isSynthetic()||!LeaseParent.class.getDeclaredMethod("access$000").isSynthetic())throw new AssertionError("native source ownership/accessor flags");LeaseParent.Link link=new LeaseParent.Link();link.peer=new LeaseA.Marker();if(link.peer.getClass()!=LeaseA.Marker.class)throw new AssertionError("joint source type binding");for(Object v:new Object[]{null,new Object()}){p.LeaseBase.set(v);if(new LeaseA.Builder().read()!=v||new LeaseParent.BuilderBase().read()!=v)throw new AssertionError("original inherited field identity");}System.out.println("source:binary:ancestor:cycle:native-flags:identity");}}`

func TestNativeAccessorAncestorSourceCycleAndBinaryViews(t *testing.T) {
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Lease", "Cove"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(prefix+"/"+debug, func(t *testing.T) {
				files := nativeAccessorAncestorCycleInput(t, prefix, debug)
				original := t.TempDir()
				for n, raw := range files {
					if e := os.MkdirAll(filepath.Dir(filepath.Join(original, n)), 0700); e != nil {
						t.Fatal(e)
					}
					if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
				run := func(cp, mode string) {
					t.Helper()
					out, e := exec.Command(java, "-Xverify:all", "-cp", cp, "q."+prefix+"Driver", mode).CombinedOutput()
					if e != nil || string(out) != "source:binary:ancestor:cycle:native-flags:identity\n" {
						t.Fatalf("original-driver %s:%v\n%s", mode, e, out)
					}
				}
				run(original, "source")
				for _, world := range []string{"source", "binary"} {
					t.Run(world, func(t *testing.T) {
						input := map[string][]byte{}
						for n, raw := range files {
							if world == "source" || n == "q/"+prefix+"A.class" || strings.HasPrefix(n, "q/"+prefix+"A$") {
								input[n] = raw
							}
						}
						jar := filepath.Join(t.TempDir(), "original.jar")
						if e := os.WriteFile(jar, t23Zip(t, input), 0600); e != nil {
							t.Fatal(e)
						}
						resolve := func(n string) ([]byte, bool) { raw, known := files[n+".class"]; return raw, known }
						z, e := NewJarFSFromLocalWithSourceVersion(jar, 8, resolve)
						if e != nil {
							t.Fatal(e)
						}
						defer z.Close()
						start := make(chan struct{})
						failures := make(chan string, 16)
						var wg sync.WaitGroup
						for i := 0; i < 16; i++ {
							wg.Add(1)
							go func(i int) {
								defer wg.Done()
								<-start
								n := "q/" + prefix + "A"
								if world == "source" && i%2 != 0 {
									n = "q/" + prefix + "Parent"
								}
								source, e := z.ReadFile(n + ".class")
								if e != nil || strings.Contains(string(source), DecompileStubMarker) || !strings.Contains(string(source), "class Builder") {
									failures <- fmt.Sprintf("cold %s: %v\n%s", n, e, source)
								}
							}(i)
						}
						close(start)
						done := make(chan struct{})
						go func() { wg.Wait(); close(done) }()
						select {
						case <-done:
						case <-time.After(15 * time.Second):
							t.Fatal("original accessor ancestor dependency did not complete before source publication")
						}
						close(failures)
						for err := range failures {
							t.Error(err)
						}
						if t.Failed() {
							return
						}
						if world == "source" {
							raw, _ := Parse(files["q/"+prefix+"A.class"])
							a := z.nativeMemberTransactionEntry(raw)
							raw, _ = Parse(files["q/"+prefix+"Parent.class"])
							b := z.nativeMemberTransactionEntry(raw)
							if a == nil || b == nil || a.family == nil || b.family == nil || len(a.family.getters) != 1 || len(b.family.getters) != 1 {
								t.Fatal("joint native accessor families missing")
							}
							snap, _ := json.Marshal(snapshotJDECEnv())
							ar := z.nativeMemberPolicyEntry("q/" + prefix + "A\x00" + string(snap))
							br := z.nativeMemberPolicyEntry("q/" + prefix + "Parent\x00" + string(snap))
							if ar.transaction == nil || ar.transaction != br.transaction || len(ar.transaction.owners) != 2 || z.nativeMembersBytes != int64(len(a.source)+len(b.source)) {
								t.Fatal("cold readers did not conserve one source transaction")
							}
						}
						out := t.TempDir()
						var sources []string
						for n := range input {
							if !(n == "q/"+prefix+"A.class" || strings.HasPrefix(n, "q/"+prefix+"A$") || world == "source" && (n == "q/"+prefix+"Parent.class" || strings.HasPrefix(n, "q/"+prefix+"Parent$") || n == "q/"+prefix+"Middle.class")) {
								continue
							}
							source, e := z.ReadFile(n)
							if e != nil || strings.Contains(string(source), DecompileStubMarker) {
								t.Fatalf("source %s:%v\n%s", n, e, source)
							}
							p := filepath.Join(out, strings.TrimSuffix(n, ".class")+".java")
							if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
								t.Fatal(e)
							}
							if e = os.WriteFile(p, source, 0600); e != nil {
								t.Fatal(e)
							}
							sources = append(sources, p)
						}
						if log, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", original, "-d", out}, sources...)...).CombinedOutput(); e != nil {
							t.Fatalf("complete source compilation:%v\n%s", e, log)
						}
						for _, n := range []string{"p/" + prefix + "Base.class", "q/" + prefix + "Driver.class"} {
							raw, e := os.ReadFile(filepath.Join(original, n))
							if e != nil || !bytes.Equal(raw, files[n]) {
								t.Fatal("original producer/driver replaced", n)
							}
						}
						run(out+string(os.PathListSeparator)+original, world)
					})
				}
			})
		}
	}
}

func nativeAccessorAncestorCycleInput(t *testing.T, prefix, debug string) map[string][]byte {
	t.Helper()
	return nativeCompileSourceReleaseClasses(t, map[string]string{"p/" + prefix + "Base.java": strings.ReplaceAll(`package p;public class LeaseBase{protected static Object value=new Object();public static void set(Object v){value=v;}}`, "Lease", prefix), "q/" + prefix + "Programs.java": strings.ReplaceAll(accessorAncestorCycleSource, "Lease", prefix)}, debug, "8")
}
