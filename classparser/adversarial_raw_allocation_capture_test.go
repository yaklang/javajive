package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const rawAllocationCaptureFixture = `
class RawAllocationScope<N> {
 interface Seen<T>{boolean add(T value);int size();}
 Seen<N> make(){final java.util.Collection<N> seen=new java.util.ArrayList<N>();return new Seen<N>(){public boolean add(N value){if(seen.contains(value))return false;return seen.add(value);}public int size(){return seen.size();}};}
}
class AllocatedCollection<T> extends java.util.ArrayList<T>{}
class RawAllocationDriver {
 public static void main(String[]args){int rows=0;Object first=new String("equal"),second=new String("equal"),identity=new Object();Object[]values={null,first,second,identity,null,identity,second};
  for(int repeat=0;repeat<3;repeat++){RawAllocationScope<Object>owner=new RawAllocationScope<Object>();RawAllocationScope.Seen<Object>seen=owner.make(),separate=owner.make();java.util.ArrayList<Object>oracle=new java.util.ArrayList<Object>();
   for(Object value:values){boolean duplicate=false;for(Object old:oracle)if(java.util.Objects.equals(old,value))duplicate=true;boolean added=seen.add(value);if(added==duplicate)throw new AssertionError("capture generic set membership");if(!duplicate)oracle.add(value);if(seen.size()!=oracle.size()||separate.size()!=0)throw new AssertionError("capture allocation identity/count");rows++;}
  }
  System.out.println(rows+":raw:allocation:capture:identity:membership");
 }
}
`

func rawAllocationCaptureShape(shape string) (string, string) {
	owner, source := "RawAllocationScope", rawAllocationCaptureFixture
	if shape == "transitive" {
		source = strings.Replace(source, "new java.util.ArrayList<N>()", "new AllocatedCollection<N>()", 1)
	}
	if shape == "renamed" {
		owner = "IndependentSetCapture"
		source = strings.ReplaceAll(source, "RawAllocationScope", owner)
		source = strings.ReplaceAll(source, "seen", "observed")
	}
	return owner, source
}

func TestAdversarialRawAllocationCaptureKeepsOriginalWideDeclaration(t *testing.T) {
	for _, shape := range []string{"direct", "transitive", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, source := rawAllocationCaptureShape(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "RawAllocationDriver", "21:raw:allocation:capture:identity:membership\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialRawAllocationCaptureOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"direct", "transitive", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, source := rawAllocationCaptureShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, javac, []string{owner}, "RawAllocationDriver", "21:raw:allocation:capture:identity:membership\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialRawAllocationCaptureOracleRejectsSemanticMutants(t *testing.T) {
	_, java := t04Tools(t)
	original := nativeCompileSourceReleaseClasses(t, map[string]string{"RawAllocationScope.java": rawAllocationCaptureFixture}, "none", "8")
	for _, mutation := range []struct{ name, before, after string }{
		{"original", "", ""},
		{"identity-instead-of-equality", "seen.contains(value)", "seen.stream().anyMatch(old -> old == value)"},
		{"missing-insertion", "return seen.add(value);", "return true;"},
		{"wrong-duplicate-result", "if(seen.contains(value))return false;", "if(seen.contains(value))return true;"},
		{"shared-allocation", "Seen<N> make(){final java.util.Collection<N> seen=new java.util.ArrayList<N>();", "private final java.util.Collection<N> shared=new java.util.ArrayList<N>(); Seen<N> make(){final java.util.Collection<N> seen=shared;"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			files := original
			if mutation.name != "original" {
				if strings.Count(rawAllocationCaptureFixture, mutation.before) != 1 {
					t.Fatal("ambiguous semantic mutation")
				}
				files = nativeCompileSourceReleaseClasses(t, map[string]string{"RawAllocationScope.java": strings.Replace(rawAllocationCaptureFixture, mutation.before, mutation.after, 1)}, "none", "8")
			}
			out := t.TempDir()
			for name, raw := range original {
				if name == "RawAllocationScope.class" || strings.HasPrefix(name, "RawAllocationScope$") {
					var ok bool
					raw, ok = files[name]
					if !ok {
						t.Fatal("mutant lost original owned class", name)
					}
				}
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, assertions := range []string{"-ea", "-da"} {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				output, err := exec.CommandContext(ctx, java, "-Xverify:all", "-Xmx128m", assertions, "-cp", out, "RawAllocationDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutation.name == "original" {
					if interrupted || err != nil || string(output) != "21:raw:allocation:capture:identity:membership\n" {
						t.Fatalf("original %s:%v %s", assertions, err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError:") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "NoClassDefFoundError") {
					t.Fatalf("compiled mutant escaped unchanged semantic driver:%v %s", err, output)
				}
			}
		})
	}
}
