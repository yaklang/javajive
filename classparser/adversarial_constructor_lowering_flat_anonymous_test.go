package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const constructorLoweringFlatAnonymousFixture = `class FlatAnonWitness {static Object expected;static FlatAnonBase escaped;static RuntimeException failure;}
abstract class FlatAnonBase {FlatAnonBase(Object input){FlatAnonWitness.escaped=this;if(value()!=FlatAnonWitness.expected)throw new AssertionError("anonymous early capture");if(input==null)throw FlatAnonWitness.failure;}abstract Object value();}
public class FlatAnonOwner {public FlatAnonBase make(Object input,final Object token){return new FlatAnonBase(input){Object value(){return token;}};}}
class FlatAnonDriver {public static void main(String[]args){int rows=0;FlatAnonOwner owner=new FlatAnonOwner();for(Object token:new Object[]{new Object(),null,"token"})for(int fail=0;fail<2;fail++){FlatAnonWitness.expected=token;FlatAnonWitness.escaped=null;FlatAnonWitness.failure=new IllegalArgumentException("shared");Throwable error=null;try{owner.make(fail==0?new Object():null,token);}catch(Throwable e){error=e;}if(error!=(fail==0?null:FlatAnonWitness.failure)||FlatAnonWitness.escaped==null||FlatAnonWitness.escaped.value()!=token||FlatAnonWitness.escaped.getClass().getModifiers()!=0)throw new AssertionError("anonymous capture/visibility/abrupt",error);rows++;}System.out.println(rows+":flat:anonymous:visibility:lifecycle");}}`

func TestAdversarialConstructorLoweringKeepsFlatAnonymousPhysicalVisibility(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, constructorLoweringFlatAnonymousFixture, "FlatAnonOwner", debug, false)
			run := func(directory string) {
				t.Helper()
				for _, assertions := range []string{"-ea", "-da"} {
					log, err := exec.Command(java, assertions, "-Xverify:all", "-cp", directory, "FlatAnonDriver").CombinedOutput()
					if err != nil || string(log) != "6:flat:anonymous:visibility:lifecycle\n" {
						t.Fatalf("independent anonymous oracle: %v\n%s", err, log)
					}
				}
			}
			run(sourceArtifactWriteFiles(t, original))
			// Only the anonymous child is owned. Its original enclosing factory
			// and binary caller stay independent; there is no source-family
			// owner to absorb its declaration or its executable constructor.
			const name = "FlatAnonOwner$1.class"
			jar := filepath.Join(t.TempDir(), "original.jar")
			if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{name: original[name]}), 0600); err != nil {
				t.Fatal(err)
			}
			a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(binary string) ([]byte, bool) { raw, ok := original[binary+".class"]; return raw, ok })
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			source, err := a.ReadFile(name)
			if err != nil || legacySourceHasFailureMarker(string(source)) || !strings.Contains(string(source), "class FlatAnonOwner$1") || strings.Contains(string(source), "public class FlatAnonOwner$1") {
				t.Fatalf("physical anonymous source header: %v\n%s", err, source)
			}
			dependencies := map[string][]byte{}
			for n, raw := range original {
				if n != name {
					dependencies[n] = raw
				}
			}
			out := sourceArtifactWriteFiles(t, dependencies)
			path := filepath.Join(out, "FlatAnonOwner$1.java")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
				t.Fatal("flat anonymous original absent from compiler", err, string(log), string(source))
			}
			fresh, err := os.ReadFile(filepath.Join(out, name))
			if err != nil {
				t.Fatal(err)
			}
			input := bytes.Clone(fresh)
			finished, err := a.FinishClass(name, fresh)
			if err != nil || !bytes.Equal(fresh, input) || mustParseLowering(t, finished).AccessFlags != mustParseLowering(t, original[name]).AccessFlags {
				t.Fatal("actual class/constructor/field ABI completion", err)
			}
			if err := os.WriteFile(filepath.Join(out, name), finished, 0600); err != nil {
				t.Fatal(err)
			}
			run(out)
		})
	}
}
