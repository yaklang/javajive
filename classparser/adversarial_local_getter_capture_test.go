package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func localGetterCaptureSources(shape string) (string, map[string]string) {
	owner, sources := factorySupertypeCaptureSources(shape)
	source := sources[owner+".java"]
	header := "class " + owner + "<N>{"
	source = strings.Replace(source, header, header+"final java.util.Comparator<? super N> order;"+owner+"(java.util.Comparator<? super N> order){this.order=order;}java.util.Comparator<? super N> ordering(){CopyFactory.getterCalls++;return order;}", 1)
	source = strings.Replace(source, "final java.util.List<N>", "final java.util.Comparator<? super N> ordering=ordering();final java.util.List<N>", 1)
	source = strings.Replace(source, "CopyFactory.copy(input)", "CopyFactory.ordered(input,ordering)", 1)
	source = strings.Replace(source, "return CopyFactory.choose(", "if(ordering==null)throw new AssertionError(\"missing original comparator\");return CopyFactory.choose(", 1)
	source = strings.Replace(source, "new "+owner+"<Object>()", "new "+owner+"<Object>(new java.util.Comparator<Object>(){public int compare(Object left,Object right){CopyFactory.compareCalls++;return 0;}})", 1)
	source = strings.Replace(source, "CopyFactory.calls=0;", "CopyFactory.calls=0;CopyFactory.getterCalls=0;CopyFactory.compareCalls=0;", 1)
	source = strings.Replace(source, "CopyFactory.calls!=1", "CopyFactory.calls!=1||CopyFactory.getterCalls!=1||(values.length>1&&CopyFactory.compareCalls==0)", 1)
	sources[owner+".java"] = source
	sources["CopyFactory.java"] = strings.Replace(sources["CopyFactory.java"], "public class CopyFactory{", `public class CopyFactory{public static int getterCalls,compareCalls;public static <T> java.util.ArrayList<T> ordered(Iterable<T> input,java.util.Comparator<? super T> order){calls++;java.util.ArrayList<T> out=new java.util.ArrayList<T>();for(T value:input)out.add(value);java.util.Collections.sort(out,order);return out;}`, 1)
	if shape == "inherited" {
		header := "class " + owner + "<N>{"
		getter := "final java.util.Comparator<? super N> order;" + owner + "(java.util.Comparator<? super N> order){this.order=order;}java.util.Comparator<? super N> ordering(){CopyFactory.getterCalls++;return order;}"
		if strings.Count(sources[owner+".java"], getter) != 1 {
			panic("authored getter declaration boundary")
		}
		sources[owner+".java"] = strings.Replace(strings.Replace(sources[owner+".java"], getter, "", 1), header, "class "+owner+"<N> extends GetterBase<N>{"+owner+"(java.util.Comparator<? super N> order){super(order);}", 1)
		sources["GetterBase.java"] = `public class GetterBase<B>{final java.util.Comparator<? super B> order;GetterBase(java.util.Comparator<? super B> order){this.order=order;}java.util.Comparator<? super B> ordering(){CopyFactory.getterCalls++;return order;}}`
	}
	return owner, sources
}

func TestAdversarialLocalGetterOracleRejectsEffectAndIdentityMutants(t *testing.T) {
	_, java := t04Tools(t)
	owner, sources := localGetterCaptureSources("direct")
	for _, mutant := range []struct{ name, file, before, after string }{
		{"original", "", "", ""},
		{"repeated getter", owner + ".java", "final java.util.Comparator<? super N> ordering=ordering();", "ordering();final java.util.Comparator<? super N> ordering=ordering();"},
		{"lost getter effect", owner + ".java", "CopyFactory.getterCalls++;return order;", "return order;"},
		{"ignored comparator", "CopyFactory.java", "java.util.Collections.sort(out,order);", ""},
		{"reordered identities", "CopyFactory.java", "return out;", "java.util.Collections.reverse(out);return out;"},
		{"wrong overload", owner + ".java", "CopyFactory.choose(copied)", "CopyFactory.choose((java.util.ArrayList<N>)copied)"},
	} {
		t.Run(mutant.name, func(t *testing.T) {
			changed := map[string]string{}
			for name, source := range sources {
				changed[name] = source
			}
			if mutant.file != "" {
				// The ordered and ordinary copy factories both return out; target
				// only the first declaration, whose original body is ordered.
				if !strings.Contains(changed[mutant.file], mutant.before) {
					t.Fatal("authored mutation target missing")
				}
				changed[mutant.file] = strings.Replace(changed[mutant.file], mutant.before, mutant.after, 1)
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
				output, err := exec.CommandContext(ctx, java, assertions, "-Xverify:all", "-cp", out, "FactoryCaptureDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutant.file == "" {
					if interrupted || err != nil || string(output) != "3:factory:projection:capture:identity:overload\n" {
						t.Fatalf("original oracle: %v %s", err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError:") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "LinkageError") {
					t.Fatalf("unchanged driver accepted effect/identity mutant: %v %s", err, output)
				}
			}
		})
	}
}

func TestAdversarialLocalGetterResultRetainsGenericCapture(t *testing.T) {
	for _, shape := range []string{"direct", "transitive", "renamed", "inherited"} {
		t.Run(shape, func(t *testing.T) {
			owner, sources := localGetterCaptureSources(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "FactoryCaptureDriver", "3:factory:projection:capture:identity:overload\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialLocalGetterResultOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"direct", "transitive", "renamed", "inherited"} {
		t.Run(shape, func(t *testing.T) {
			owner, sources := localGetterCaptureSources(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return localGetterCompileJavac8(t, sources, debug)
			}, NativeJavac8, javac, []string{owner}, "FactoryCaptureDriver", "3:factory:projection:capture:identity:overload\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func localGetterCompileJavac8(t *testing.T, sources map[string]string, debug string) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	paths := []string{}
	for name, source := range sources {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	args := append([]string{"-proc:none", "-g:" + debug, "-d", dir}, paths...)
	if output, err := exec.Command(os.Getenv("JAVA8_JAVAC"), args...).CombinedOutput(); err != nil {
		t.Fatalf("original javac8: %v %s", err, output)
	}

	result := map[string][]byte{}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".class") {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = raw
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	return result
}
