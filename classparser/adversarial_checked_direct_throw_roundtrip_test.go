package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The original checked object can escape through ATHROW without an invoke at
// all. Removing the optional declaration preserves the verified JVM behavior.
func TestAdversarialCheckedDirectThrowIdentityRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `public class CheckedDirectThrowReview {
 static final java.io.IOException failure=new java.io.IOException("identity");
 static void direct(java.io.IOException value)throws java.io.IOException{throw value;}
 public static void main(String[] args)throws java.io.IOException{try{direct(failure);}catch(Throwable caught){System.out.println(caught==failure);}try{direct(null);}catch(NullPointerException caught){System.out.println("null");}}
}`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "CheckedDirectThrowReview.java")
			if err := os.WriteFile(file, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
				t.Fatalf("original: %v\n%s", err, out)
			}
			want := t04RunJava(t, java, dir, "CheckedDirectThrowReview")
			if strings.TrimSpace(want) != "true\nnull" {
				t.Fatalf("independent identity/null oracle %q", want)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "CheckedDirectThrowReview.class"))
			if err != nil {
				t.Fatal(err)
			}
			object, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			removed := 0
			for _, method := range object.Methods {
				name, _ := object.getUtf8(method.NameIndex)
				if name != "direct" {
					continue
				}
				var attrs []AttributeInfo
				for _, attr := range method.Attributes {
					if _, ok := attr.(*ExceptionsAttribute); ok {
						removed++
						continue
					}
					attrs = append(attrs, attr)
				}
				method.Attributes = attrs
			}
			if removed != 1 {
				t.Fatalf("caller declaration removals %d", removed)
			}
			raw = object.Bytes()
			if err := os.WriteFile(filepath.Join(dir, "CheckedDirectThrowReview.class"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if got := t04RunJava(t, java, dir, "CheckedDirectThrowReview"); got != want {
				t.Fatalf("metadata mutation changed JVM oracle: %q want %q", got, want)
			}
			resolve := func(name string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
				return b, e == nil
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					var result DecompileResult
					var e error
					if mode == "legacy" {
						result.Source, e = DecompileWithResolver(raw, resolve)
					} else {
						result, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolve})
					}
					if e != nil {
						t.Fatal(e)
					}
					if len(result.StubMethods) > 0 {
						t.Fatalf("stub: %v", result.StubMethods)
					}
					rebuilt := t.TempDir()
					path := filepath.Join(rebuilt, "CheckedDirectThrowReview.java")
					if e = os.WriteFile(path, []byte(result.Source), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-d", rebuilt, path).CombinedOutput(); e != nil {
						t.Fatalf("rebuild: %v\n%s\n%s", e, out, result.Source)
					}
					if got := t04RunJava(t, java, rebuilt, "CheckedDirectThrowReview"); got != want {
						t.Fatalf("got %q want %q\n%s", got, want, result.Source)
					}
				})
			}
		})
	}
}
