package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The unchanged binary driver compares actual String/exception values and
// lexical identity. Source marker spelling is deliberately not its oracle.
const nativeDiagnosticDataFixture = `
class LiteralOwner {
 int calls;
 String value(int x){calls++;switch(x&3){case 0:return "empty slot value";case 1:return "yak-decompiler:";case 2:return "yak-decompiler-internal: try without catch handler";default:return "= Exception;";}}
 class Inner {String value(int x){return LiteralOwner.this.value(x);}}
 Inner member(){return new Inner();}
 java.util.concurrent.Callable<String> anonymous(final int x){return new java.util.concurrent.Callable<String>(){public String call(){return "yak-decompiler:"+value(x);}};}
 java.util.function.Supplier<String> lambda(final int x){return ()->"empty slot value"+value(x);}
 String checked(int x){assert x>=0:"yak-decompiler:";return value(x);}
 String abrupt(){throw new IllegalArgumentException("empty slot value");}
}
class LiteralDriver {public static void main(String[]args)throws Exception {
 String[] expected={"empty slot value","yak-decompiler:","yak-decompiler-internal: try without catch handler","= Exception;"};LiteralOwner o=new LiteralOwner();int rows=0;
 for(int x=-128;x<=128;x++){String e=expected[x&3];int before=o.calls;if(!o.value(x).equals(e)||o.calls!=before+1)throw new AssertionError("plain data/evaluation");
 LiteralOwner.Inner member=o.member();if(member.getClass().getDeclaringClass()!=LiteralOwner.class||!member.value(x).equals(e)||o.calls!=before+2)throw new AssertionError("member data/binding");
 java.util.concurrent.Callable<String> a=o.anonymous(x);if(!a.getClass().isAnonymousClass()||a.getClass().getEnclosingMethod().getDeclaringClass()!=LiteralOwner.class||!a.call().equals(expected[1]+e)||o.calls!=before+3)throw new AssertionError("anonymous data/binding");
 if(!o.lambda(x).get().equals(expected[0]+e)||o.calls!=before+4)throw new AssertionError("lambda data/binding");
 try{String checked=o.checked(x);if(x<0||!checked.equals(e)||o.calls!=before+5)throw new AssertionError("assert continuation");}catch(AssertionError failure){if(x>=0||!expected[1].equals(failure.getMessage())||o.calls!=before+4)throw failure;}
 try{o.abrupt();throw new AssertionError("missing abrupt");}catch(IllegalArgumentException failure){if(!failure.getMessage().equals(expected[0]))throw failure;}rows++;}
 System.out.println(rows+":literal:identity:order:scope");
}}
`

func TestNativeDiagnosticDataRoundTrip(t *testing.T) { testNativeDiagnosticDataRoundTrip(t, false) }
func TestNativeDiagnosticDataNativeRoundTrip(t *testing.T) {
	testNativeDiagnosticDataRoundTrip(t, true)
}
func testNativeDiagnosticDataRoundTrip(t *testing.T, native bool) {
	javac, java := t04Tools(t)
	profile := ModernJavac
	if native {
		javac = os.Getenv("JAVA8_JAVAC")
		if javac == "" {
			t.Fatal("JAVA8_JAVAC required")
		}
		profile = NativeJavac8
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			var files map[string][]byte
			if native {
				files = nativePrivateEnumCompile(t, nativeDiagnosticDataFixture, "LiteralOwner", debug)
			} else {
				files = nativeCompileDebugClasses(t, nativeDiagnosticDataFixture, debug)
			}
			write := func(dir, name string, b []byte) {
				t.Helper()
				p := filepath.Join(dir, name)
				if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(p, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			original := t.TempDir()
			for n, b := range files {
				write(original, n, b)
			}
			run := func(dir string) string {
				t.Helper()
				out, e := exec.Command(java, "-ea", "-cp", dir, "LiteralDriver").CombinedOutput()
				if e != nil {
					t.Fatalf("driver %v: %s", e, out)
				}
				return string(out)
			}
			const want = "257:literal:identity:order:scope\n"
			if got := run(original); got != want {
				t.Fatalf("original oracle: %q", got)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					jar := filepath.Join(t.TempDir(), "input.jar")
					if e := os.WriteFile(jar, t23Zip(t, files), 0600); e != nil {
						t.Fatal(e)
					}
					z, e := NewJarFSFromLocalWithCompilerProfile(jar, 8, profile, nil)
					if e != nil {
						t.Fatal(e)
					}
					defer z.Close()
					child, e := z.ReadFile("LiteralOwner$Inner.class")
					if e != nil || !strings.Contains(string(child), "original member body owned by") {
						t.Fatalf("joint ownership: %v %s", e, child)
					}
					src, e := z.ReadFile("LiteralOwner.class")
					if e != nil {
						t.Fatal(e)
					}
					output := t.TempDir()
					for n, b := range files {
						if n == "LiteralOwner.class" || strings.HasPrefix(n, "LiteralOwner$") {
							continue
						}
						write(output, n, b)
					}
					write(output, "LiteralOwner.java", src)
					args := []string{"-proc:none", "-g:" + debug, "-cp", output, "-d", output}
					if native {
						args = append(args, "-source", "8", "-target", "8")
					} else {
						args = append(args, "--release", "8")
					}
					args = append(args, filepath.Join(output, "LiteralOwner.java"))
					if out, e := exec.Command(javac, args...).CombinedOutput(); e != nil {
						t.Fatalf("recompile: %v %s\n%s", e, out, src)
					}
					rebuilt := map[string]bool{}
					if e := filepath.Walk(output, func(p string, i os.FileInfo, e error) error {
						if e != nil {
							return e
						}
						if i.IsDir() || !strings.HasSuffix(p, ".class") {
							return nil
						}
						rel, e := filepath.Rel(output, p)
						if e != nil {
							return e
						}
						rel = filepath.ToSlash(rel)
						if rel != "LiteralOwner.class" && !strings.HasPrefix(rel, "LiteralOwner$") {
							return nil
						}
						b, e := os.ReadFile(p)
						if e != nil {
							return e
						}
						old, ok := files[rel]
						if !ok {
							t.Fatalf("invented target declaration %s", rel)
						}
						if nativeBinaryShape(t, old) != nativeBinaryShape(t, b) {
							t.Fatalf("original ABI changed: %s", rel)
						}
						nativeLexicalExactSignatures(t, rel, old, b)
						rebuilt[rel] = true
						return nil
					}); e != nil {
						t.Fatal(e)
					}
					for n := range files {
						if (n == "LiteralOwner.class" || strings.HasPrefix(n, "LiteralOwner$")) && !rebuilt[n] {
							t.Fatalf("missing original target %s", n)
						}
					}
					if got := run(output); got != want {
						t.Fatalf("candidate oracle: %q", got)
					}
				})
			}
		})
	}
}
