package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Both original targets are private, so equal erased signatures in related
// owners do not override each other. Reconstructed nest helpers must preserve
// that separation even though they have package access in flattened sources.
func TestAdversarialPrivateNestInheritedBridgeNamesRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const sameNest = `
public class NestInheritedBridgeReview {
 static class Base { private Object first(Object value){return value;} }
 static class Child extends Base { private Object second(Object value){return value;} }
 static class Reader {
  static Object readBase(Base owner,Object value){return owner.first(value);}
  static Object readChild(Child owner,Object value){return owner.second(value);}
 }
 public static void main(String[] args){Object token=new Object();Child owner=new Child();System.out.println(Reader.readBase(owner,token)==token);System.out.println(Reader.readChild(owner,token)==token);}
}
`
	const crossNest = `
public class NestInheritedBridgeReview {
 static class Base { private Object first(Object value){return value;} }
 static class Reader {static Object readBase(Base owner,Object value){return owner.first(value);}}
 public static void main(String[] args){Object token=new Object();ForeignNestBridgeHost.Child owner=new ForeignNestBridgeHost.Child();System.out.println(Reader.readBase(owner,token)==token);System.out.println(ForeignNestBridgeHost.Reader.readChild(owner,token)==token);}
}
class ForeignNestBridgeHost {
 static class Child extends NestInheritedBridgeReview.Base {private Object second(Object value){return value;}}
 static class Reader {static Object readChild(Child owner,Object value){return owner.second(value);}}
}
`
	// A real original member occupies the owner-derived candidate. It is not
	// the private target and must never be called or overwritten by a bridge.
	collisionNest := strings.Replace(sameNest, "static class Base {", fmt.Sprintf("static class Base { final Object jdec$private$%x$0(Object value){return new Object();}", []byte("NestInheritedBridgeReview$Base")), 1)
	for _, fixture := range []struct{ name, source string }{{"same-nest", sameNest}, {"cross-nest", crossNest}, {"original-name-collision", collisionNest}} {
		source := fixture.source
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(fixture.name+"/"+debug, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "NestInheritedBridgeReview.java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "11", debug, "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatalf("original: %v\n%s", err, out)
				}
				want := t04RunJava(t, java, dir, "NestInheritedBridgeReview")
				if strings.TrimSpace(want) != "true\ntrue" {
					t.Fatalf("independent oracle: %q", want)
				}
				resolve := func(name string) ([]byte, bool) {
					b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
					return b, e == nil
				}
				units, err := filepath.Glob(filepath.Join(dir, "*.class"))
				if err != nil {
					t.Fatal(err)
				}
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					t.Run(string(mode), func(t *testing.T) {
						rebuilt := t.TempDir()
						args := []string{"-proc:none", "--release", "11", "-d", rebuilt}
						allSource := ""
						for _, unit := range units {
							raw, e := os.ReadFile(unit)
							if e != nil {
								t.Fatal(e)
							}
							var result DecompileResult
							if mode == "legacy" {
								result.Source, e = DecompileWithResolver(raw, resolve)
							} else {
								result, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolve})
							}
							if e != nil {
								t.Fatal(e)
							}
							if len(result.StubMethods) > 0 {
								t.Fatalf("%s stub: %v", unit, result.StubMethods)
							}
							name := strings.TrimSuffix(filepath.Base(unit), ".class")
							p := filepath.Join(rebuilt, name+".java")
							if e = os.WriteFile(p, []byte(result.Source), 0600); e != nil {
								t.Fatal(e)
							}
							args = append(args, p)
							allSource += result.Source
						}
						if out, e := exec.Command(javac, args...).CombinedOutput(); e != nil {
							t.Fatalf("rebuild: %v\n%s\n%s", e, out, allSource)
						}
						if got := t04RunJava(t, java, rebuilt, "NestInheritedBridgeReview"); got != want {
							t.Fatalf("got %q want %q\n%s", got, want, allSource)
						}
					})
				}
			})
		}
	}
}
