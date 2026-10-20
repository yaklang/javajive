package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const independentStandaloneCompletionFixture = `
class GenericBoundary { static class MetadataOnly{} GenericPeer.Token peer;
 public enum State { LIVE, MISSING }
 public interface Loaded<T> {
  T resolve(Class<T> type);
  State state();
  abstract class AbstractBase<V> implements Loaded<V> {
   public State state(){return State.LIVE;}
   public static abstract class ForMissing<W> extends AbstractBase<W> {
    public W resolve(Class<W> type){throw new IllegalStateException(type.getName());}
   }
  }
 }
}
class GenericPeer { static class Token{} GenericBoundary.Loaded<?> peer; }
class GenericValue<T> extends GenericBoundary.Loaded.AbstractBase<T> {
 final T value;
 GenericValue(T value){this.value=value;}
 public T resolve(Class<T> type){return type.cast(value);}
}
class GenericMissing<T> extends GenericBoundary.Loaded.AbstractBase.ForMissing<T> {}
class GenericDriver {
 public static void main(String[] args) {
  int rows=0;String[] shared=new String[]{"same"};
  for(String[] value:new String[][]{null,shared,new String[0],new String[]{"same"}}){
   GenericBoundary.Loaded<String[]> loaded=new GenericValue<String[]>(value);
   if(loaded.resolve(String[].class)!=value||loaded.state()!=GenericBoundary.State.LIVE)throw new AssertionError("generic binding or state identity");
   try{loaded.resolve(null);throw new AssertionError("lost null class check");}catch(NullPointerException expected){}
   try{Object result=((GenericBoundary.Loaded)loaded).resolve((Class)Integer.class);if(value!=null||result!=null)throw new AssertionError("lost runtime class cast");}catch(ClassCastException expected){if(value==null)throw new AssertionError("wrong null cast");}
   GenericBoundary.Loaded<String[]> missing=new GenericMissing<String[]>();
   try{missing.resolve(String[].class);throw new AssertionError("lost exception");}catch(IllegalStateException expected){if(!expected.getMessage().equals("[Ljava.lang.String;"))throw new AssertionError("wrong class binding");}
   if(missing.state()!=GenericBoundary.State.LIVE||missing.getClass().getSuperclass()!=GenericBoundary.Loaded.AbstractBase.ForMissing.class)throw new AssertionError("nested base ABI");
   if(GenericBoundary.Loaded.AbstractBase.class.getDeclaringClass()!=GenericBoundary.Loaded.class||GenericBoundary.Loaded.AbstractBase.ForMissing.class.getDeclaringClass()!=GenericBoundary.Loaded.AbstractBase.class)throw new AssertionError("nested ownership");
   rows++;
  }
  System.out.println(rows+":generic:standalone:completion");
 }
}
`

func TestAdversarialIndependentStandaloneCompletionKeepsGenericNestedBodies(t *testing.T) {
	for _, omit := range []bool{false, true} {
		name := "complete archive"
		if omit {
			name = "unrelated declaration unavailable"
		}
		t.Run(name, func(t *testing.T) { testIndependentStandaloneCompletion(t, omit) })
	}
}
func testIndependentStandaloneCompletion(t *testing.T, omit bool) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 compiler")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, independentStandaloneCompletionFixture, "GenericBoundary", debug, true)
			// Mixed-version archives are valid. The outer is metadata only for
			// this independently proved Java 8 boundary, not a source owner.
			outer, err := Parse(files["GenericBoundary.class"])
			if err != nil {
				t.Fatal(err)
			}
			outer.MajorVersion = 55
			files["GenericBoundary.class"] = outer.Bytes()
			// An unused declaration outside the proved source boundary need not
			// exist at runtime; it must not become our lexical or private scope.
			if omit {
				delete(files, "GenericBoundary$MetadataOnly.class")
			}
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, "GenericDriver")
			if want != "4:generic:standalone:completion\n" {
				t.Fatal("original oracle", want)
			}
			for _, reverse := range []bool{false, true} {
				label := "root-first"
				if reverse {
					label = "child-first"
				}
				t.Run(label, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					if _, _, known := z.nativeMemberDependencyAdmission("GenericBoundary$Loaded", nil, snapshotJDECEnv()); omit && known {
						t.Fatal("fixture did not leave the larger graph unclosed")
					}
					if reverse {
						if _, err := z.ReadFile("GenericBoundary$Loaded$AbstractBase$ForMissing.class"); err != nil {
							t.Fatal(err)
						}
					}
					stateSource, err := z.ReadFile("GenericBoundary$State.class")
					if err != nil {
						t.Fatal(err)
					}
					source, err := z.ReadFile("GenericBoundary$Loaded.class")
					if err != nil || strings.Contains(string(source), DecompileStubMarker) || !strings.Contains(string(source), "class AbstractBase") || !strings.Contains(string(source), "class ForMissing") {
						t.Fatalf("completed standalone boundary lost its nested bodies: %v\n%s", err, source)
					}
					out := t.TempDir()
					owned := map[string][]byte{}
					for name, raw := range files {
						if strings.HasPrefix(name, "GenericBoundary$Loaded") || name == "GenericBoundary$State.class" {
							owned[name] = raw
							continue
						}
						if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if len(owned) != 4 {
						t.Fatal("original complete boundary", len(owned))
					}
					path := filepath.Join(out, "GenericBoundary$Loaded.java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					statePath := filepath.Join(out, "GenericBoundary$State.java")
					if err := os.WriteFile(statePath, stateSource, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-parameters", "-g:"+debug, "-cp", out, "-d", out, path, statePath).CombinedOutput(); err != nil {
						t.Fatalf("compile complete boundary without original fallback: %v\n%s\n%s", err, log, source)
					}
					if got := t04RunJava(t, java, out, "GenericDriver"); got != want {
						t.Fatalf("same unchanged binary caller: %q wanted %q", got, want)
					}
					for name, before := range files {
						after, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						if owned[name] == nil {
							if string(before) != string(after) {
								t.Fatal("unowned original changed", name)
							}
						} else if name == "GenericBoundary$Loaded.class" || name == "GenericBoundary$State.class" {
							nativeAnonymousPrefixRepresentationShape(t, name, before, after)
							nativeLexicalExactSignatures(t, name, before, after)
						} else {
							nativeLexicalExactSignatures(t, name, before, after)
							if a, b := nativeBinaryShape(t, before), nativeBinaryShape(t, after); a != b {
								t.Fatalf("exact nested generic ABI %s\n%s\n%s", name, a, b)
							}
							if a, b := nativeAnonymousAccessorShape(t, before), nativeAnonymousAccessorShape(t, after); a != b {
								t.Fatalf("exact nested ownership %s\n%s\n%s", name, a, b)
							}
						}
					}
				})
			}
		})
	}
}
