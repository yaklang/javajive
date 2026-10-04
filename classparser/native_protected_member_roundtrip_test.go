package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMemberRestorationRetainsForeignEnclosingAccess(t *testing.T) {
	javac, java := t04Tools(t)
	for _, visibility := range []string{"public", "protected"} {
		t.Run(visibility, func(t *testing.T) {
			for _, debug := range []string{"-g", "-g:none"} {
				t.Run(debug, func(t *testing.T) {
					original := t.TempDir()
					sourceFiles := map[string]string{
						"p/AccessOwner.java":  fmt.Sprintf(`package p;public class AccessOwner{%s static class Token{protected Token(){}public long read(){return 7;}}}`, visibility),
						"q/ForeignOwner.java": `package q;public class ForeignOwner extends p.AccessOwner{protected static class Child extends Token{public long read(){return super.read()+3;}}public static long check(){return new Child().read();}}`,
						"q/DirectOwner.java":  `package q;public class DirectOwner extends p.AccessOwner{public static Token token;public static long read(Token n){return n==null?0:n.read();}}`,
						"q/AccessDriver.java": `package q;public class AccessDriver{public static void main(String[]args){long n=ForeignOwner.check();if(n!=10)throw new AssertionError("enclosing access");System.out.println(n);}}`,
					}
					paths := []string{}
					for n, s := range sourceFiles {
						p := filepath.Join(original, n)
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, []byte(s), 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", debug, "-d", original}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("original %v %s", e, out)
					}
					oracle := t04RunJava(t, java, original, "q.AccessDriver")
					files := map[string][]byte{}
					if e := filepath.Walk(original, func(p string, info os.FileInfo, e error) error {
						if e != nil {
							return e
						}
						if info.IsDir() || !strings.HasSuffix(p, ".class") {
							return nil
						}
						n, e := filepath.Rel(original, p)
						if e != nil {
							return e
						}
						b, e := os.ReadFile(p)
						if e != nil {
							return e
						}
						files[filepath.ToSlash(n)] = b
						return nil
					}); e != nil {
						t.Fatal(e)
					}
					// A direct subclass has its own protected access. Removing the foreign
					// nested child leaves a representable family; do not blanket-ban it.
					directFiles := map[string][]byte{}
					for n, b := range files {
						if n != "q/ForeignOwner.class" && n != "q/ForeignOwner$Child.class" && n != "q/AccessDriver.class" {
							directFiles[n] = b
						}
					}
					direct := nativeArchive(t, directFiles)
					directSource, e := direct.ReadFile("p/AccessOwner.class")
					if e != nil || !strings.Contains(string(directSource), "class Token") {
						t.Fatalf("direct protected subtype lost ownership %v %s", e, directSource)
					}
					if visibility == "protected" {
						for _, scenario := range []string{"original", "cycle", "missing", "wrong identity", "canceled"} {
							t.Run("access-proof-"+scenario, func(t *testing.T) {
								inputs := map[string][]byte{}
								for n, b := range directFiles {
									inputs[n] = b
								}
								if scenario == "missing" {
									delete(inputs, "q/DirectOwner.class")
								}
								if scenario == "cycle" || scenario == "wrong identity" {
									obj, e := Parse(append([]byte(nil), inputs["q/DirectOwner.class"]...))
									if e != nil {
										t.Fatal(e)
									}
									if scenario == "cycle" {
										obj.SuperClass = obj.ThisClass
									} else {
										obj.ThisClass = obj.SuperClass
									}
									inputs["q/DirectOwner.class"] = obj.Bytes()
								}
								z := nativeArchive(t, inputs)
								owner, e := Parse(inputs["p/AccessOwner.class"])
								if e != nil {
									t.Fatal(e)
								}
								family := z.nativeMemberReader(owner).planNativeMemberFamily()
								if family == nil {
									t.Fatal("fixture ownership proof missing")
								}
								index := &nativeMemberIndex{typeUsers: map[string]map[string]bool{"p/AccessOwner$Token": {"q/DirectOwner": true}}}
								var work *workbudget.Budget
								if scenario == "canceled" {
									ctx, cancel := context.WithCancel(context.Background())
									cancel()
									work = workbudget.New(ctx, workbudget.Limits{})
								}
								if got := z.nativeMemberAccessRepresentable(family, index, work); got != (scenario == "original") {
									t.Fatalf("access proof %s = %v", scenario, got)
								}
							})
						}
					}
					for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(mode, func(t *testing.T) {
							if mode == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if mode == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							z := nativeArchive(t, files)
							rebuilt := t.TempDir()
							paths := []string{}
							// Preserve the independently compiled caller, then regenerate both owners
							// and every child source unit. Access cannot be inferred from binary '$'.
							for n, b := range files {
								p := filepath.Join(rebuilt, filepath.FromSlash(n))
								if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
									t.Fatal(e)
								}
								if n == "q/AccessDriver.class" {
									if e := os.WriteFile(p, b, 0600); e != nil {
										t.Fatal(e)
									}
									continue
								}
								src, e := z.ReadFile(n)
								if e != nil || strings.Contains(string(src), DecompileStubMarker) {
									t.Fatalf("source %s %v %s", n, e, src)
								}
								p = strings.TrimSuffix(p, ".class") + ".java"
								if e := os.WriteFile(p, src, 0600); e != nil {
									t.Fatal(e)
								}
								paths = append(paths, p)
							}
							if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt}, paths...)...).CombinedOutput(); e != nil {
								t.Fatalf("rebuilt %v %s", e, out)
							}
							for n, b := range files {
								if n == "q/AccessDriver.class" {
									continue
								}
								got, e := os.ReadFile(filepath.Join(rebuilt, filepath.FromSlash(n)))
								if e != nil {
									t.Fatal(e)
								}
								if want, actual := nativeBinaryShape(t, b), nativeBinaryShape(t, got); want != actual {
									t.Fatalf("ABI %s\n%s\n!=\n%s", n, want, actual)
								}
							}
							if got := t04RunJava(t, java, rebuilt, "q.AccessDriver"); got != oracle {
								t.Fatalf("%q != %q", got, oracle)
							}
						})
					}
				})
			}
		})
	}
}
