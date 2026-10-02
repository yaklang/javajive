package javaclassparser

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A target archive may inherit helpers and invoke checked producers declared
// only in dependencies. Source compilation needs those original declarations;
// omitting them is incomplete, not permission to invent collision-free names.
func TestAdversarialJarDeclarationResolverRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `class JarDeclarationParent { public static RuntimeException jdec$rethrow$0(Throwable failure){return new IllegalStateException(failure);} }
class JarDeclarationActions {static final java.io.IOException failure=new java.io.IOException("identity");static Object action()throws java.io.IOException{throw failure;}}
public class JarDeclarationOwner extends JarDeclarationParent {Object invoke()throws java.io.IOException{return JarDeclarationActions.action();} public static void main(String[]args){try{new JarDeclarationOwner().invoke();}catch(Throwable caught){System.out.println(caught==JarDeclarationActions.failure);}}}`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "JarDeclarationOwner.java")
			if err := os.WriteFile(file, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
				t.Fatalf("original: %v\n%s", err, out)
			}
			classes := classMapFromDir(t, dir)
			obj, err := Parse(classes["JarDeclarationOwner"])
			if err != nil {
				t.Fatal(err)
			}
			removed := 0
			for _, method := range obj.Methods {
				name, _ := obj.getUtf8(method.NameIndex)
				if name != "invoke" {
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
				t.Fatal("fixture lacks exact Exceptions declaration")
			}
			classes["JarDeclarationOwner"] = obj.Bytes()
			if err := os.WriteFile(filepath.Join(dir, "JarDeclarationOwner.class"), obj.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if got := t04RunJava(t, java, dir, "JarDeclarationOwner"); got != "true\n" {
				t.Fatalf("original identity %q", got)
			}
			var target bytes.Buffer
			w := zip.NewWriter(&target)
			entry, err := w.Create("JarDeclarationOwner.class")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(obj.Bytes()); err != nil {
				t.Fatal(err)
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			jar := filepath.Join(dir, "target.jar")
			if err = os.WriteFile(jar, target.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			for _, supplied := range []bool{false, true} {
				resolver := func(name string) ([]byte, bool) {
					// A dependency cannot shadow a target's own declaration.
					if name == "JarDeclarationOwner" {
						t.Fatal("target delegated to dependency resolver")
					}
					if !supplied && name == "JarDeclarationParent" {
						return nil, false
					}
					return resolverFromClasses(classes)(name)
				}
				fs, err := NewJarFSFromLocalWithResolver(jar, resolver)
				if err != nil {
					t.Fatal(err)
				}
				own, ok := fs.enumSiblingResolver()("JarDeclarationOwner")
				if !ok || !bytes.Equal(own, obj.Bytes()) {
					t.Fatal("dependency replaced target declaration")
				}
				b, err := fs.ReadFile("JarDeclarationOwner.class")
				if supplied {
					var outer bytes.Buffer
					zw := zip.NewWriter(&outer)
					ze, e := zw.Create("nested.jar")
					if e != nil {
						t.Fatal(e)
					}
					if _, e = ze.Write(target.Bytes()); e != nil {
						t.Fatal(e)
					}
					if e = zw.Close(); e != nil {
						t.Fatal(e)
					}
					outerPath := filepath.Join(dir, "outer.jar")
					if e = os.WriteFile(outerPath, outer.Bytes(), 0600); e != nil {
						t.Fatal(e)
					}
					nested, e := NewJarFSFromLocalWithResolver(outerPath, resolver)
					if e != nil {
						t.Fatal(e)
					}
					inner, e := nested.ReadFile("nested.jar/JarDeclarationOwner.class")
					if closeErr := nested.Close(); closeErr != nil {
						t.Fatal(closeErr)
					}
					if e != nil || !bytes.Equal(inner, b) {
						t.Fatalf("nested declarations changed source: %v\n%s", e, inner)
					}
				}
				if closeErr := fs.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				if err != nil {
					t.Fatal(err)
				}
				if !supplied {
					if !strings.Contains(string(b), "checked escape helper requires complete inherited member names") {
						t.Fatalf("missing namespace accepted:\n%s", b)
					}
					continue
				}
				if strings.Contains(string(b), "yak-decompiler:") {
					t.Fatalf("complete namespace stubbed:\n%s", b)
				}
				rebuilt := t.TempDir()
				file := filepath.Join(rebuilt, "JarDeclarationOwner.java")
				if err = os.WriteFile(file, b, 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, file).CombinedOutput(); err != nil {
					t.Fatalf("rebuild: %v\n%s\n%s", err, out, b)
				}
				if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "JarDeclarationOwner"); got != "true\n" {
					t.Fatalf("rebuilt exception identity %q\n%s", got, b)
				}
			}
		})
	}
}
