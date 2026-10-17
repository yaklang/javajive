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

func TestAdversarialAnonymousLexicalOracleRejectsMutants(t *testing.T) {
	_, java := t04Tools(t)
	owner, sources := anonymousLexicalRewriteSources(false)
	for _, mutant := range []struct{ name, replacement string }{
		{"original", ""},
		{"lost effect", "return (V)key;"},
		{"repeated effect", "function.apply(key);return function.apply(key);"},
		{"lost identity", "return function.apply(null);"},
		{"changed exception identity", "try{return function.apply(key);}catch(IllegalStateException caught){throw new IllegalStateException(caught.getMessage());}"},
	} {
		t.Run(mutant.name, func(t *testing.T) {
			changed := map[string]string{}
			for name, source := range sources {
				changed[name] = source
			}
			if mutant.replacement != "" {
				changed[owner+".java"] = strings.Replace(changed[owner+".java"], "return function.apply(key);", mutant.replacement, 1)
			}
			files := nativeCompileSourceReleaseClasses(t, changed, "none", "8")
			out := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, assertions := range []string{"-ea", "-da"} {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				output, err := exec.CommandContext(ctx, java, assertions, "-Xverify:all", "-cp", out, "LexicalFunctionDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutant.replacement == "" {
					if interrupted || err != nil || string(output) != "lexical:identity:null:effect:exception:sibling\n" {
						t.Fatalf("original oracle: %v %s", err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError:") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "LinkageError") {
					t.Fatalf("unchanged driver accepted mutant: %v %s", err, output)
				}
			}
		})
	}
}

func TestNativeClassRecoveryCannotBorrowAnonymousMemberBinding(t *testing.T) {
	_, java := t04Tools(t)
	for _, renamed := range []bool{false, true} {
		owner, sources := anonymousLexicalRewriteSources(renamed)
		// Use the native renderer's ownership marker. The payload itself is
		// independently compilable Java, with two disjoint lexical type scopes.
		source := sources[owner+".java"]
		for _, ordinal := range []string{"1", "2"} {
			source = strings.Replace(source, "return new Probe", "return /*jdec-owned-anonymous-ordinal:"+ordinal+":"+owner+"*/new Probe", 1)
		}
		rule := func(s string) string { return fixRxjavaRemainingWithBinding(s, true) }
		unowned := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}}
		if unowned.sourceRewrite("fixRxjavaRemainingReconstructs", "class_source", source, rule) == source {
			t.Fatal("fixture did not exercise the legacy receiver-binding recovery")
		}
		for _, phase := range []string{"class_source", "method_source"} {
			t.Run(owner+"/"+phase, func(t *testing.T) {
				d := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}, nativeAnonymousRoot: &nativeAnonymousFamily{}}
				rebuiltSource := d.sourceRewrite("fixRxjavaRemainingReconstructs", phase, source, rule)
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(debug, func(t *testing.T) {
						for _, compiler := range []string{"modern", "original8"} {
							t.Run(compiler, func(t *testing.T) {
								if compiler == "original8" && os.Getenv("JAVA8_JAVAC") == "" {
									t.Skip("JAVA8_JAVAC required for actual compiler")
								}
								compile := func(input map[string]string) map[string][]byte {
									if compiler == "original8" {
										return localGetterCompileJavac8(t, input, debug)
									}
									return nativeCompileSourceReleaseClasses(t, input, debug, "8")
								}
								original := compile(sources)
								changed := map[string]string{}
								for name, text := range sources {
									changed[name] = text
								}
								changed[owner+".java"] = rebuiltSource
								rebuilt := compile(changed)
								for name, raw := range original {
									if nativeBinaryShape(t, raw) != nativeBinaryShape(t, rebuilt[name]) {
										t.Fatal("source recovery changed physical ABI", name)
									}
									nativeLexicalExactSignatures(t, name, raw, rebuilt[name])
								}
								for _, files := range []map[string][]byte{original, rebuilt} {
									out := t.TempDir()
									for name, raw := range files {
										if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
											t.Fatal(err)
										}
									}
									if got := t04RunJava(t, java, out, "LexicalFunctionDriver"); got != "lexical:identity:null:effect:exception:sibling\n" {
										t.Fatalf("independent runtime oracle: %q", got)
									}
								}
							})
						}
					})
				}
			})
		}
	}
}

// Two independent generic methods deliberately reuse a receiver name. A
// declaration in one method supplies no type evidence for the other's member.
func anonymousLexicalRewriteSources(renamed bool) (string, map[string]string) {
	owner := "LexicalFunctionOwner"
	sources := map[string]string{
		"Probe.java": `public abstract class Probe<A,B>{public abstract B apply(A value);}`,
		owner + ".java": `import java.util.function.Function;public class LexicalFunctionOwner{
 static <K,V> Probe<K,V> make(final Function<? super K,V> function){
  return new Probe<K,V>(){public V apply(K key){return function.apply(key);}};
 }
 static <V1,V2> Probe<V1,V2> other(final Function<? super V1,V2> function){
  return new Probe<V1,V2>(){public V2 apply(V1 key){return function.apply(key);}};
 }
}`,
		"LexicalFunctionDriver.java": `import java.util.function.Function;public class LexicalFunctionDriver{
 static int calls;static final Object token=new Object();static final Object poison=new Object();
 static final IllegalStateException failure=new IllegalStateException("original");
 public static void main(String[] args){
  Function<Object,Object> function=new Function<Object,Object>(){public Object apply(Object value){calls++;if(value==poison)throw failure;return value;}};
  Probe<Object,Object> probe=LexicalFunctionOwner.make(function);
  if(probe.apply(token)!=token||probe.apply(null)!=null||calls!=2)throw new AssertionError("identity or effect");
  try{probe.apply(poison);throw new AssertionError("missing failure");}catch(IllegalStateException caught){if(caught!=failure||calls!=3)throw new AssertionError("exception identity");}
  if(LexicalFunctionOwner.other(function).apply(token)!=token||calls!=4)throw new AssertionError("sibling method");
  System.out.println("lexical:identity:null:effect:exception:sibling");
 }
}`,
	}
	if renamed {
		replace := strings.NewReplacer("LexicalFunctionOwner", "RenamedLexicalOwner", " function", " conversion", "(function)", "(conversion)", "function.apply", "conversion.apply", "V1", "SiblingInput", "V2", "SiblingOutput", "<K,V>", "<Input,Output>", " K ", " Input ", " V ", " Output ", "? super K,V", "? super Input,Output", "(K key)", "(Input key)")
		changed := map[string]string{}
		for name, source := range sources {
			changed[replace.Replace(name)] = replace.Replace(source)
		}
		sources, owner = changed, "RenamedLexicalOwner"
	}
	return owner, sources
}

func TestAdversarialAnonymousClassRewriteRetainsLexicalBinding(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		owner, sources := anonymousLexicalRewriteSources(renamed)
		t.Run(owner, func(t *testing.T) {
			testNativePrivateSetterCompiledFixture(t, owner, "LexicalFunctionDriver", "lexical:identity:null:effect:exception:sibling\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousClassRewriteOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, renamed := range []bool{false, true} {
		owner, sources := anonymousLexicalRewriteSources(renamed)
		t.Run(owner, func(t *testing.T) {
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return localGetterCompileJavac8(t, sources, debug)
			}, NativeJavac8, javac, []string{owner}, "LexicalFunctionDriver", "lexical:identity:null:effect:exception:sibling\n", nil, nativeLexicalExactSignatures)
		})
	}
}
