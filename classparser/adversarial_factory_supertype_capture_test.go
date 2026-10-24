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

func factorySupertypeCaptureSources(shape string) (string, map[string]string) {
	owner := "FactoryCaptureOwner"
	source := `class FactoryCaptureOwner<N>{interface Snapshot<M>{M get(int index);int size();String chosen();}Snapshot<N> make(java.util.Collection<N> input){final java.util.List<N> copied=CopyFactory.copy(input);return new Snapshot<N>(){public N get(int index){return copied.get(index);}public int size(){return copied.size();}public String chosen(){return CopyFactory.choose(copied);}};}}
class FactoryCaptureDriver{public static void main(String[]args)throws Exception{Object first=new String("same"),second=new String("same"),token=new Object();Object[][] cases={{},{null},{first,second,token,null,first}};int rows=0;for(Object[] values:cases){java.util.ArrayList<Object> input=new java.util.ArrayList<Object>();for(Object value:values)input.add(value);CopyFactory.calls=0;FactoryCaptureOwner.Snapshot<Object> snapshot=new FactoryCaptureOwner<Object>().make(input);input.clear();if(CopyFactory.calls!=1||snapshot.size()!=values.length||!snapshot.chosen().equals("wide"))throw new AssertionError("factory identity/count/overload");for(int i=0;i<values.length;i++)if(snapshot.get(i)!=values[i])throw new AssertionError("snapshot element identity");java.lang.reflect.Field field=snapshot.getClass().getDeclaredField("val$copied");if(field.getType()!=java.util.List.class)throw new AssertionError("original capture declaration");rows++;}System.out.println(rows+":factory:projection:capture:identity:overload");}}`
	if shape == "transitive" {
		source = strings.Replace(source, "make(java.util.Collection<N> input)", "make(java.util.ArrayList<N> input)", 1)
	}
	if shape == "renamed" {
		owner = "InheritedFactorySnapshot"
		source = strings.ReplaceAll(source, "FactoryCaptureOwner", owner)
		source = strings.ReplaceAll(source, "copied", "retained")
	}
	return owner, map[string]string{
		owner + ".java":    source,
		"CopyFactory.java": `public class CopyFactory{public static int calls;public static <T> java.util.ArrayList<T> copy(Iterable<T> input){calls++;java.util.ArrayList<T> out=new java.util.ArrayList<T>();for(T value:input)out.add(value);return out;}public static String choose(java.util.List<?> value){return "wide";}public static String choose(java.util.ArrayList<?> value){return "narrow";}}`,
	}
}

func TestAdversarialFactorySupertypeCapturePreservesOriginalBinding(t *testing.T) {
	for _, shape := range []string{"direct", "transitive", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, sources := factorySupertypeCaptureSources(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "FactoryCaptureDriver", "3:factory:projection:capture:identity:overload\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialFactorySupertypeCaptureOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"direct", "transitive", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, sources := factorySupertypeCaptureSources(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, sources[owner+".java"]+sources["CopyFactory.java"], "CopyFactory", debug)
			}, NativeJavac8, javac, []string{owner}, "FactoryCaptureDriver", "3:factory:projection:capture:identity:overload\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialFactorySupertypeCaptureOracleRejectsSemanticMutants(t *testing.T) {
	_, java := t04Tools(t)
	owner, sources := factorySupertypeCaptureSources("direct")
	original := nativeCompileSourceReleaseClasses(t, sources, "none", "8")
	for _, mutation := range []struct{ name, before, after string }{
		{"original", "", ""},
		{"changed element identity", "return copied.get(index);", "return (N)new Object();"},
		{"changed overload", "CopyFactory.choose(copied)", "CopyFactory.choose((java.util.ArrayList<N>)copied)"},
		{"repeated factory", "final java.util.List<N> copied=", "CopyFactory.copy(input);final java.util.List<N> copied="},
		{"lost snapshot", "CopyFactory.copy(input);return", "(java.util.List<N>)input;return"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			files := original
			if mutation.before != "" {
				changed := make(map[string]string, len(sources))
				for name, source := range sources {
					changed[name] = source
				}
				if strings.Count(changed[owner+".java"], mutation.before) != 1 {
					t.Fatal("ambiguous semantic mutation")
				}
				changed[owner+".java"] = strings.Replace(changed[owner+".java"], mutation.before, mutation.after, 1)
				files = nativeCompileSourceReleaseClasses(t, changed, "none", "8")
			}
			out := t.TempDir()
			for name, raw := range original {
				if name == owner+".class" || strings.HasPrefix(name, owner+"$") {
					var found bool
					raw, found = files[name]
					if !found {
						t.Fatal("mutant lost owned class", name)
					}
				}
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, assertions := range []string{"-ea", "-da"} {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				output, err := exec.CommandContext(ctx, java, assertions, "-Xverify:all", "-cp", out, "FactoryCaptureDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutation.before == "" {
					if interrupted || err != nil || string(output) != "3:factory:projection:capture:identity:overload\n" {
						t.Fatalf("original driver: %v %s", err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError:") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "LinkageError") {
					t.Fatalf("unchanged driver did not reject semantic mutant: %v %s", err, output)
				}
			}
		})
	}
}
