package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Attach an original checked declaration to an independently compiled factory.
// Only the Exceptions attribute changes; the verified JVM and its initializer
// error/once semantics are unchanged. Interface and annotation helpers must
// propagate the same Throwable, just as the class initializer does.
func TestAdversarialOriginalCheckedInitializerRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `
class OriginalInitCheckedActions {
 static int trace;static final java.io.IOException failure=new java.io.IOException("init");
 static <E extends Throwable> Object raise(Throwable failure)throws E {throw (E)failure;}
 static Object make(boolean fail){trace++;if(fail)return OriginalInitCheckedActions.<RuntimeException>raise(failure);return "value"+trace;}
 static void declaration()throws java.io.IOException{}
 static void report(Throwable caught){System.out.println(caught.getClass().getName()+":"+(caught.getCause()==failure)+":"+trace);}
}
interface CheckedInitInterface {Object VALUE=OriginalInitCheckedActions.make(false);}
@interface CheckedInitAnnotation {Object VALUE=OriginalInitCheckedActions.make(false);}
class CheckedInitClass {static Object VALUE=OriginalInitCheckedActions.make(true);}
public class OriginalCheckedInitializerReview {
 public static void main(String[]args){System.out.println(CheckedInitInterface.VALUE);System.out.println(CheckedInitInterface.VALUE==CheckedInitInterface.VALUE);System.out.println(CheckedInitAnnotation.VALUE);System.out.println(CheckedInitAnnotation.VALUE==CheckedInitAnnotation.VALUE);try{System.out.println(CheckedInitClass.VALUE);}catch(Throwable caught){OriginalInitCheckedActions.report(caught);}try{System.out.println(CheckedInitClass.VALUE);}catch(Throwable caught){OriginalInitCheckedActions.report(caught);}System.out.println(OriginalInitCheckedActions.trace);}
}`
	for _, debug := range []string{"-g", "-g:none"} {
		dir := t.TempDir()
		file := filepath.Join(dir, "OriginalCheckedInitializerReview.java")
		if err := os.WriteFile(file, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
			t.Fatalf("original compile %v\n%s", err, out)
		}
		want := t04RunJava(t, java, dir, "OriginalCheckedInitializerReview")
		factoryFile := filepath.Join(dir, "OriginalInitCheckedActions.class")
		raw, err := os.ReadFile(factoryFile)
		if err != nil {
			t.Fatal(err)
		}
		factory, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var declaration *ExceptionsAttribute
		for _, method := range factory.Methods {
			name, _ := factory.getUtf8(method.NameIndex)
			if name == "declaration" {
				for _, attribute := range method.Attributes {
					if attr, ok := attribute.(*ExceptionsAttribute); ok {
						declaration = attr
					}
				}
			}
		}
		if declaration == nil {
			t.Fatal("missing original checked declaration")
		}
		for _, method := range factory.Methods {
			name, _ := factory.getUtf8(method.NameIndex)
			if name == "make" {
				method.Attributes = append(method.Attributes, declaration)
			}
		}
		if err := os.WriteFile(factoryFile, factory.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		if got := t04RunJava(t, java, dir, "OriginalCheckedInitializerReview"); got != want {
			t.Fatalf("attribute mutation changed verified oracle: got%q want%q", got, want)
		}
		resolve := func(name string) ([]byte, bool) {
			data, err := os.ReadFile(filepath.Join(dir, name+".class"))
			return data, err == nil
		}
		for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
			rebuilt := t.TempDir()
			var files []string
			for _, name := range []string{"OriginalCheckedInitializerReview", "CheckedInitInterface", "CheckedInitAnnotation", "CheckedInitClass"} {
				data, ok := resolve(name)
				if !ok {
					t.Fatal(name)
				}
				var result DecompileResult
				if mode == "legacy" {
					result.Source, err = DecompileWithResolver(data, resolve)
				} else {
					result, err = DecompileWithOptions(data, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
				}
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(rebuilt, name+".java")
				if err := os.WriteFile(path, []byte(result.Source), 0644); err != nil {
					t.Fatal(err)
				}
				files = append(files, path)
			}
			args := append([]string{"-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt}, files...)
			if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
				for _, file := range files {
					data, _ := os.ReadFile(file)
					t.Logf("%s\n%s", file, data)
				}
				t.Fatalf("rebuild %s/%s %v\n%s", mode, debug, err, out)
			}
			if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "OriginalCheckedInitializerReview"); got != want {
				t.Fatalf("%s/%s got%q want%q", mode, debug, got, want)
			}
		}
	}
}
