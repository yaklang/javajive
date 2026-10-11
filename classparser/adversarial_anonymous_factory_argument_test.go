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

// A Java anonymous creation expression has its declared parent source type;
// its computational anonymous binary name is not a raw generic argument.
func anonymousFactoryArgumentSources(shape string) (string, map[string]string) {
	owner, sources := factorySupertypeCaptureSources(shape)
	sources[owner+".java"] = strings.Replace(sources[owner+".java"], "CopyFactory.copy(input)", "CopyFactory.transform(input,new CopyFactory.Mapper<N,N>(){public N apply(N value){return value;}})", 1)
	sources["CopyFactory.java"] = strings.Replace(sources["CopyFactory.java"], "public class CopyFactory{", `public class CopyFactory{public interface Mapper<I,O>{O apply(I value);}public static <I,O> java.util.ArrayList<O> transform(Iterable<I> input,Mapper<? super I,? extends O> mapper){calls++;java.util.ArrayList<O> out=new java.util.ArrayList<O>();for(I value:input)out.add(mapper.apply(value));return out;}`, 1)
	if shape == "bounded" {
		source := strings.Replace(sources[owner+".java"], "class FactoryCaptureOwner<N>", "class FactoryCaptureOwner<N extends java.io.Serializable>", 1)
		before, driver, found := strings.Cut(source, "class FactoryCaptureDriver")
		if !found {
			panic("authored driver boundary")
		}
		driver = strings.ReplaceAll(driver, "new Object()", "new String(\"token\")")
		driver = strings.ReplaceAll(driver, "Object", "java.io.Serializable")
		sources[owner+".java"] = before + "class FactoryCaptureDriver" + driver
	}
	return owner, sources
}

func TestAdversarialAnonymousFactoryArgumentKeepsLexicalType(t *testing.T) {
	for _, shape := range []string{"direct", "transitive", "renamed", "bounded"} {
		t.Run(shape, func(t *testing.T) {
			owner, sources := anonymousFactoryArgumentSources(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "FactoryCaptureDriver", "3:factory:projection:capture:identity:overload\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousFactoryArgumentOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"direct", "transitive", "renamed", "bounded"} {
		t.Run(shape, func(t *testing.T) {
			owner, sources := anonymousFactoryArgumentSources(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, sources[owner+".java"]+sources["CopyFactory.java"], "CopyFactory", debug)
			}, NativeJavac8, javac, []string{owner}, "FactoryCaptureDriver", "3:factory:projection:capture:identity:overload\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousFactoryArgumentOracleRejectsSemanticMutants(t *testing.T) {
	_, java := t04Tools(t)
	owner, sources := anonymousFactoryArgumentSources("direct")
	original := nativeCompileSourceReleaseClasses(t, sources, "none", "8")
	for _, mutation := range []struct{ name, before, after string }{
		{"original", "", ""},
		{"lost mapper identity", "return value;", "return null;"},
		{"changed captured identity", "return copied.get(index);", "return (N)new Object();"},
		{"wrong overload", "CopyFactory.choose(copied)", "CopyFactory.choose((java.util.ArrayList<N>)copied)"},
		{"repeated producer", "final java.util.List<N> copied=", "CopyFactory.copy(input);final java.util.List<N> copied="},
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
