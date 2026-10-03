package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Removing caller Exceptions attributes does not change JVM execution. It does
// require an exact source escape bridge, including names inherited across JDK
// modules. The checked payload and side effects remain independently observed.
func TestAdversarialModularCheckedEscapeNamespacesRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `
class ModularEscapeActions {static int calls;static final java.io.IOException failure=new java.io.IOException("identity");static Object action()throws java.io.IOException{calls++;throw failure;}}
class ModularEscapeXml extends org.xml.sax.helpers.DefaultHandler {static Object invoke()throws java.io.IOException{return ModularEscapeActions.action();}static int jdec$rethrow$0(){return 0;}}
class ModularEscapeBeans extends java.beans.PropertyEditorSupport {static Object invoke()throws java.io.IOException{return ModularEscapeActions.action();}}
abstract class ModularEscapeSql implements javax.sql.DataSource {static Object invoke()throws java.io.IOException{return ModularEscapeActions.action();}}
public class ModularEscapeDriver {public static void main(String[]args){for(int i=0;i<3;i++){try{if(i==0)ModularEscapeXml.invoke();else if(i==1)ModularEscapeBeans.invoke();else ModularEscapeSql.invoke();throw new AssertionError("no throw");}catch(Throwable failure){if(failure!=ModularEscapeActions.failure||ModularEscapeActions.calls!=i+1)throw new AssertionError("payload/effects",failure);System.out.println("same:"+ModularEscapeActions.calls);}}}}
`
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "ModularEscapeDriver.java")
			if err := os.WriteFile(file, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
				t.Fatalf("original: %v\n%s", err, out)
			}
			want := t04RunJava(t, java, dir, "ModularEscapeDriver")
			names := []string{"ModularEscapeXml", "ModularEscapeBeans", "ModularEscapeSql"}
			original := map[string][]byte{}
			for _, name := range names {
				obj, err := Parse(readClassBytes(t, dir, name))
				if err != nil {
					t.Fatal(err)
				}
				removed := 0
				for _, m := range obj.Methods {
					n, _ := obj.getUtf8(m.NameIndex)
					if n != "invoke" {
						continue
					}
					var attrs []AttributeInfo
					for _, a := range m.Attributes {
						if _, ok := a.(*ExceptionsAttribute); ok {
							removed++
							continue
						}
						attrs = append(attrs, a)
					}
					m.Attributes = attrs
				}
				if removed != 1 {
					t.Fatalf("%s: removed%d Exceptions attributes", name, removed)
				}
				original[name] = obj.Bytes()
				if err := os.WriteFile(filepath.Join(dir, name+".class"), original[name], 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, dir, "ModularEscapeDriver"); got != want {
				t.Fatalf("original metadata mutation changed JVM oracle: %q want%q", got, want)
			}
			resolve := func(name string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
				return b, e == nil
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					rebuilt := t.TempDir()
					args := []string{"-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt}
					for _, name := range names {
						var result DecompileResult
						var err error
						if mode == "legacy" {
							result.Source, err = DecompileWithResolver(original[name], resolve)
						} else {
							result, err = DecompileWithOptions(original[name], DecompileOptions{Mode: mode, Resolve: resolve})
						}
						if err != nil || len(result.StubMethods) > 0 {
							t.Fatalf("%s: error=%v stubs=%v\n%s", name, err, result.StubMethods, result.Source)
						}
						path := filepath.Join(rebuilt, name+".java")
						if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
							t.Fatal(err)
						}
						args = append(args, path)
					}
					if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt: %v\n%s", err, out)
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "ModularEscapeDriver"); got != want {
						t.Fatalf("rebuilt oracle: %q want%q", got, want)
					}
				})
			}
		})
	}
}
