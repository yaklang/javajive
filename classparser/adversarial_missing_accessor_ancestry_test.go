package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAdversarialUnavailableAccessorAncestryCannotPoisonIndependentSource(t *testing.T) {
	javac, java := t04Tools(t)
	for _, prefix := range []string{"Native", "Cove"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, available := range []bool{false, true} {
				mode := "missing external declarations"
				if available {
					mode = "complete external declarations"
				}
				t.Run(prefix+"/"+debug+"/"+mode, func(t *testing.T) {
					fixture := strings.Replace(nativeOwnerFixture, "class NativeOwner {", "class NativeOwner {static class Link extends Producer.BuilderBase{}", 1)
					fixture = strings.Replace(fixture, "public static void main(String[]x){", `public static void main(String[]x){if(NativeOwner.Link.class.getDeclaringClass()!=NativeOwner.class||new Producer.BuilderBase().read()!=Producer.current())throw new AssertionError("unrelated source lost scope or original inherited storage");`, 1)
					fixture += `class Producer extends p.HiddenBase{private static Object value=new Object();static Object current(){return value;}static class BuilderBase{Object read(){return Producer.value;}}}`
					fixture = strings.ReplaceAll(fixture, "Native", prefix)
					files := nativeCompileSourceReleaseClasses(t, map[string]string{prefix + "Driver.java": fixture, "p/HiddenBase.java": `package p;public class HiddenBase{}`}, debug, "8")
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
					want := "18:I" + strings.Repeat("P", 18) + "\n"
					run := func(cp string) {
						t.Helper()
						out, e := exec.Command(java, "-Xverify:all", "-cp", cp, prefix+"Driver").CombinedOutput()
						if e != nil || string(out) != want {
							t.Fatalf("original-driver:%v\n%s want=%s", e, out, want)
						}
					}
					run(original)
					input := map[string][]byte{}
					for n, raw := range files {
						if n != "p/HiddenBase.class" {
							input[n] = raw
						}
					}
					jar := filepath.Join(t.TempDir(), "original.jar")
					write(filepath.Dir(jar), filepath.Base(jar), t23Zip(t, input))
					resolve := func(n string) ([]byte, bool) {
						if available && n == "p/HiddenBase" {
							return files["p/HiddenBase.class"], true
						}
						return nil, false
					}
					z, e := NewJarFSFromLocalWithSourceVersion(jar, 8, resolve)
					if e != nil {
						t.Fatal(e)
					}
					defer z.Close()
					start := make(chan struct{})
					failures := make(chan string, 16)
					var readers sync.WaitGroup
					for i := 0; i < 16; i++ {
						readers.Add(1)
						go func(i int) {
							defer readers.Done()
							<-start
							name := prefix + "Owner.class"
							if i%2 != 0 {
								name = "Producer.class"
							}
							source, e := z.ReadFile(name)
							if e != nil || len(source) == 0 || strings.Contains(string(source), DecompileStubMarker) {
								failures <- fmt.Sprintf("cold source %s:%v\n%s", name, e, source)
							}
						}(i)
					}
					close(start)
					done := make(chan struct{})
					go func() { readers.Wait(); close(done) }()
					select {
					case <-done:
					case <-time.After(15 * time.Second):
						t.Fatal("independent cold source readers did not finish")
					}
					close(failures)
					for failure := range failures {
						t.Error(failure)
					}
					if t.Failed() {
						return
					}
					out := t.TempDir()
					var paths, names []string
					for n := range files {
						names = append(names, n)
					}
					sort.Strings(names)
					for _, n := range names {
						if n != prefix+"Owner.class" && !strings.HasPrefix(n, prefix+"Owner$") && n != "Producer.class" && !strings.HasPrefix(n, "Producer$") {
							write(out, n, files[n])
							continue
						}
						source, e := z.ReadFile(n)
						if e != nil || strings.Contains(string(source), DecompileStubMarker) {
							t.Fatalf("source %s:%v\n%s", n, e, source)
						}
						p := strings.TrimSuffix(n, ".class") + ".java"
						write(out, p, source)
						paths = append(paths, filepath.Join(out, p))
					}
					if log, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("complete source compilation:%v\n%s", e, log)
					}
					for n, raw := range files {
						if n == prefix+"Owner.class" || strings.HasPrefix(n, prefix+"Owner$") || n == "Producer.class" || strings.HasPrefix(n, "Producer$") {
							continue
						}
						got, e := os.ReadFile(filepath.Join(out, n))
						if e != nil || !bytes.Equal(got, raw) {
							t.Fatal("original parent/effects/driver bytes replaced", n)
						}
					}
					run(out)
				})
			}
		}
	}
}
