package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func functionalFixture(t *testing.T, main, source, debug string) (string, []byte) {
	t.Helper()
	javac, java := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, main+".java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", dir, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile %s %s: %v\n%s", main, debug, err, out)
	}
	classBytes, err := os.ReadFile(filepath.Join(dir, main+".class"))
	if err != nil {
		t.Fatal(err)
	}
	return t04RunJava(t, java, dir, main), classBytes
}

// The LambdaMetafactory instantiated descriptor records the erasure of Entry<K,V>.
// Materializing the lambda as a local must not make that erased parameterization
// incompatible with the generic ConcurrentHashMap.computeIfPresent declaration.
func TestMaterializedBiFunctionErasureRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Parallel()
	const main = "FunctionalErasureCall"
	const source = `import java.util.AbstractMap;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.BiFunction;

public class FunctionalErasureCall<K, V> {
  private final ConcurrentHashMap<Object, Map.Entry<K, V>> data = new ConcurrentHashMap<>();
  public V refresh(Object key) {
    BiFunction<Object, Map.Entry<K, V>, Map.Entry<K, V>> updater = (k, entry) -> entry;
    return data.computeIfPresent(key, updater).getValue();
  }
  public V compute(Object key) {
    BiFunction<Object, Map.Entry<K, V>, Map.Entry<K, V>> updater = (k, entry) -> entry;
    return data.compute(key, updater).getValue();
  }
  public V merge(Object key, Map.Entry<K, V> next) {
    BiFunction<Map.Entry<K, V>, Map.Entry<K, V>, Map.Entry<K, V>> updater = (oldValue, newValue) -> oldValue;
    return data.merge(key, next, updater).getValue();
  }
  public V replaceAll(Object key) {
    BiFunction<Object, Map.Entry<K, V>, Map.Entry<K, V>> updater = (k, entry) -> entry;
    data.replaceAll(updater);
    return data.get(key).getValue();
  }
  public static void main(String[] args) {
    FunctionalErasureCall<String, Integer> cache = new FunctionalErasureCall<>();
    cache.data.put("answer", new AbstractMap.SimpleEntry<>("answer", 42));
    System.out.print(cache.refresh("answer") + "," + cache.compute("answer") + "," +
      cache.merge("answer", new AbstractMap.SimpleEntry<>("answer", 100)) + "," +
      cache.replaceAll("answer"));
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, "computeIfPresent") || !strings.Contains(result.Source, "BiFunction") {
				t.Fatalf("fixture lost generic functional call in %s/%s:\n%s", mode, debug, result.Source)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("materialized BiFunction %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// A class generic method carries the same erased SAM problem even when the
// argument is a local and the callee is declared by the current class.
func TestSameClassBiFunctionTypeVariablesRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "SameClassBiFunctionCall"
	const source = `import java.util.function.BiFunction;

public class SameClassBiFunctionCall<K, V> {
  private BiFunction<? super K, ? super V, ? extends V> installed;
  private void install(BiFunction<? super K, ? super V, ? extends V> f) {
    this.installed = f;
  }
  public void prepare() {
    BiFunction<K, V, V> f = (key, value) -> value;
    install(f);
  }
  public V apply(K key, V value) { return installed.apply(key, value); }
  public static void main(String[] args) {
    SameClassBiFunctionCall<String, Integer> cache = new SameClassBiFunctionCall<>();
    cache.prepare();
    System.out.print(cache.apply("answer", 42));
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, "BiFunction") || !strings.Contains(result.Source, "install(") {
				t.Fatalf("fixture lost same-class generic call in %s/%s:\n%s", mode, debug, result.Source)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("same-class BiFunction %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// Function<T,R> loses T in the instantiated SAM descriptor, while
// Supplier<Iterator<Entry<T,R>>> loses the nested Iterator arguments. Both
// shapes appear in Caffeine's same-class helper calls.
func TestSameClassFunctionAndSupplierErasureRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Setenv("JDEC_POLY_CALL_TARGET_OFF", "")
	const main = "FunctionalErasureVariants"
	const source = `import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import java.util.function.Function;
import java.util.function.Supplier;

public class FunctionalErasureVariants<T> {
  private final Map<T, String> values = new HashMap<>();
  private Function<? super T, ? extends T> mapper;
  private Supplier<Iterator<Map.Entry<T, String>>> supplier;
  private void installMapper(Function<? super T, ? extends T> f) { mapper = f; }
  private void installSupplier(Supplier<Iterator<Map.Entry<T, String>>> f) { supplier = f; }
  public void prepare() {
    Function<T, T> f = key -> key;
    installMapper(f);
    Supplier<Iterator<Map.Entry<T, String>>> entries = () -> values.entrySet().iterator();
    installSupplier(entries);
  }
  public String result(T key) { return values.get(mapper.apply(key)) + ":" + supplier.get().next().getValue(); }
  public static void main(String[] args) {
    FunctionalErasureVariants<Integer> cache = new FunctionalErasureVariants<>();
    cache.values.put(7, "seven");
    cache.prepare();
    System.out.print(cache.result(7));
  }
}`
	var killSwitchFixture []byte
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		if debug == "-g" {
			killSwitchFixture = classBytes
		}
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, "installMapper(") || !strings.Contains(result.Source, "installSupplier(") {
				t.Fatalf("fixture lost generic helper calls in %s/%s:\n%s", mode, debug, result.Source)
			}
			for _, declaration := range []string{
				"Function<? super T, ? extends T>",
				"Supplier<Iterator<Map.Entry<T, String>>>",
			} {
				if !strings.Contains(result.Source, declaration) {
					t.Fatalf("call target did not reach materialized local %q in %s/%s:\n%s", declaration, mode, debug, result.Source)
				}
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("Function/Supplier erasure %s/%s: %v", mode, debug, err)
			}
		}
	}

	t.Setenv("JDEC_POLY_CALL_TARGET_OFF", "1")
	legacy, err := DecompileWithOptions(killSwitchFixture, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	if err != nil {
		t.Fatal(err)
	}
	for _, erased := range []string{"Function<Object, Object>", "Supplier<Iterator>"} {
		if !strings.Contains(legacy.Source, erased) {
			t.Fatalf("call-target kill switch did not restore %q:\n%s", erased, legacy.Source)
		}
	}
}

// JDK receiver signatures also target materialized poly expressions. Map and
// Spliterator descriptors carry only raw BiConsumer/Consumer, but their
// receiver arguments prove the complete source target without inspecting a
// library-specific class or source spelling.
func TestT19JDKConsumerCallTargetsRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Setenv("JDEC_POLY_CALL_TARGET_OFF", "")
	const main = "JdkConsumerTargets"
	const source = `import java.util.LinkedHashMap;
import java.util.Map;
import java.util.function.BiConsumer;
import java.util.function.Consumer;

public class JdkConsumerTargets<K, V> {
  private final Map<K, V> values = new LinkedHashMap<>();
  void copy(Map<K, V> input) {
    BiConsumer<? super K, ? super V> put = (key, value) -> values.put(key, value);
    input.forEach(put);
  }
  String entries() {
    StringBuilder out = new StringBuilder();
    Consumer<? super Map.Entry<K, V>> append = entry -> out.append(entry.getKey()).append('=').append(entry.getValue());
    values.entrySet().spliterator().forEachRemaining(append);
    return out.toString();
  }
  public static void main(String[] args) {
    JdkConsumerTargets<String, Integer> target = new JdkConsumerTargets<>();
    Map<String, Integer> input = new LinkedHashMap<>();
    input.put("answer", 42);
    target.copy(input);
    System.out.print(target.entries());
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			for _, declaration := range []string{
				"BiConsumer<? super K, ? super V>",
				"Consumer<Map.Entry>",
			} {
				if !strings.Contains(result.Source, declaration) {
					t.Fatalf("JDK target did not reach local %q in %s/%s:\n%s", declaration, mode, debug, result.Source)
				}
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("JDK consumer target %s/%s: %v\n%s", mode, debug, err, result.Source)
			}
		}
	}
}

// A local web whose only definition is an invokedynamic lambda still has a
// source-level target type at its ARETURN use.  The classfile descriptor erases
// Function<T,R> to Function and the lambda's instantiated descriptor to
// (Object)Object; keeping that provisional Function<Object,Object> declaration
// makes it impossible to return from a method whose Signature says
// Function<? super T,? extends R>.  The return constraint must refine the web's
// parameterization while preserving the same JVM erasure.
func TestT19ReturnedFunctionalWebUsesGenericReturnConstraint(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Setenv("JDEC_GENERIC_USE_CONSTRAINT_OFF", "")
	const main = "ReturnedFunctionalWeb"
	const source = `import java.util.function.BiFunction;
import java.util.function.Function;

public class ReturnedFunctionalWeb {
  static <T, R> Function<? super T, ? extends R> measured(
      Function<? super T, ? extends R> delegate) {
    Function<? super T, ? extends R> result = value -> delegate.apply(value);
    return result;
  }

  static <T, U, R> BiFunction<? super T, ? super U, ? extends R> measured(
      BiFunction<? super T, ? super U, ? extends R> delegate) {
    BiFunction<? super T, ? super U, ? extends R> result =
        (left, right) -> delegate.apply(left, right);
    return result;
  }

  public static void main(String[] args) {
    Function<? super String, ? extends Integer> one = measured(String::length);
    BiFunction<? super String, ? super Integer, ? extends String> two =
        measured((text, count) -> text.substring(0, count));
    System.out.print(one.apply("four") + ":" + two.apply("value", 3));
  }
}`
	var killSwitchFixture []byte
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		if debug == "-g" {
			killSwitchFixture = classBytes
		}
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("returned functional web %s/%s: %v\n%s", mode, debug, err, result.Source)
			}
		}
	}

	t.Setenv("JDEC_GENERIC_USE_CONSTRAINT_OFF", "1")
	legacy, err := DecompileWithOptions(killSwitchFixture, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"Function<Object, Object>", "BiFunction<Object, Object, Object>"} {
		if !strings.Contains(legacy.Source, raw) {
			t.Fatalf("kill switch did not restore erased functional declaration %q:\n%s", raw, legacy.Source)
		}
	}
}

func TestT19LocalCacheReturnedFunctionalWebUsesGenericSignature(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/LocalCache.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_GENERIC_USE_CONSTRAINT_OFF", "")
	t.Setenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF", "")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range []string{
		"Function<? super T, ? extends R> var6 =",
		"BiFunction<? super T, ? super U, ? extends R> var10 =",
	} {
		if !strings.Contains(on, decl) {
			t.Fatalf("generic return constraint did not reach Caffeine declaration %q:\n%s", decl, on)
		}
	}
	if got := strings.Count(on, "return (R) ("); got < 2 {
		t.Fatalf("covariant functional return target did not reach both lambda bodies (got %d casts):\n%s", got, on)
	}

	// The outer Signature carries `? extends R`, while invokedynamic retains only an
	// Object-returning SAM descriptor. Disabling the return-target recovery must remove the
	// load-bearing casts without relying on the former Caffeine-specific source rewrite.
	t.Setenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF", "1")
	uncast, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(uncast, "return (R) (") {
		t.Fatalf("lambda return-target kill switch did not remove the recovered casts:\n%s", uncast)
	}

	t.Setenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF", "")
	t.Setenv("JDEC_GENERIC_USE_CONSTRAINT_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range []string{
		"Function<Object, Object> var6 =",
		"BiFunction<Object, Object, Object> var10 =",
	} {
		if !strings.Contains(off, decl) {
			t.Fatalf("kill switch did not restore Caffeine's erased declaration %q:\n%s", decl, off)
		}
	}
}

// A call into another class in the same JAR cannot use the caller's local
// MethodSignatures table. The sibling resolver must walk the parameterized
// receiver declaration, substitute its K/V arguments, and recover the full
// functional formal before deciding that a raw-erasure bridge is necessary.
func TestSiblingClassFunctionalErasureRoundTrip(t *testing.T) {
	const main = "SiblingFunctionalErasureCall"
	const source = `import java.util.concurrent.CompletableFuture;
import java.util.function.BiFunction;

final class SiblingLocalCache<K, V> {
  V compute(K key, BiFunction<? super K, ? super V, ? extends V> remap,
      boolean recordStats, boolean recordLoad, boolean notifyWriter) {
    return remap.apply(key, null);
  }
}

public class SiblingFunctionalErasureCall<K, V> {
  private final SiblingLocalCache<K, CompletableFuture<V>> delegate = new SiblingLocalCache<>();

  @SuppressWarnings({"rawtypes", "unchecked"})
  CompletableFuture<V> update(K key, V next) {
    BiFunction<Object, CompletableFuture, CompletableFuture> remap =
        (ignored, oldValue) -> oldValue == null ? CompletableFuture.completedFuture(next) : oldValue;
    return (CompletableFuture<V>) delegate.compute(key, (BiFunction) remap, false, false, false);
  }

  public static void main(String[] args) {
    System.out.print(new SiblingFunctionalErasureCall<String, Integer>().update("answer", 42).join());
  }
}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"-g", "-g:none"} {
		originalDir := t.TempDir()
		sourcePath := filepath.Join(originalDir, main+".java")
		if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", originalDir, sourcePath)
		if out, err := compile.CombinedOutput(); err != nil {
			t.Fatalf("compile %s %s: %v\n%s", main, debug, err, out)
		}
		want := t04RunJava(t, java, originalDir, main)
		classBytes, err := os.ReadFile(filepath.Join(originalDir, main+".class"))
		if err != nil {
			t.Fatal(err)
		}
		resolver := func(internalName string) ([]byte, bool) {
			data, readErr := os.ReadFile(filepath.Join(originalDir, filepath.FromSlash(internalName)+".class"))
			return data, readErr == nil
		}
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{
				Mode: mode, TargetSourceVersion: 8, Resolve: resolver,
			})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, ".compute(") || !strings.Contains(result.Source, "(BiFunction)(") {
				t.Fatalf("missing sibling-signature raw bridge in %s/%s:\n%s", mode, debug, result.Source)
			}

			rebuiltDir := t.TempDir()
			rebuiltSource := filepath.Join(rebuiltDir, main+".java")
			if err := os.WriteFile(rebuiltSource, []byte(result.Source), 0o644); err != nil {
				t.Fatal(err)
			}
			rebuild := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8",
				"-cp", originalDir, "-d", rebuiltDir, rebuiltSource)
			if out, err := rebuild.CombinedOutput(); err != nil {
				t.Fatalf("rebuild %s/%s: %v\n%s\nsource:\n%s", mode, debug, err, out, result.Source)
			}
			classpath := rebuiltDir + string(os.PathListSeparator) + originalDir
			if got := t04RunJava(t, java, classpath, main); got != want {
				t.Fatalf("runtime mismatch %s/%s: got %q want %q", mode, debug, got, want)
			}

			off, err := DecompileWithOptions(classBytes, DecompileOptions{
				Mode: mode, TargetSourceVersion: 8, Resolve: resolver,
				EnvSnapshot: map[string]string{"JDEC_FUNCTIONAL_ERASURE_RESOLVE_OFF": "1"},
			})
			if err != nil {
				t.Fatalf("decompile kill switch %s/%s: %v", mode, debug, err)
			}
			if strings.Contains(off.Source, "(BiFunction)(") {
				t.Fatalf("kill switch retained sibling-signature bridge in %s/%s:\n%s", mode, debug, off.Source)
			}
		}
	}
}

// The outer invocation in owner.cache().compute(...) sees only cache()'s erased
// descriptor return. Recovering compute's formal therefore requires two sound
// substitutions: instantiate cache() from the parameterized owner receiver, then
// use that parameterized return as compute's receiver. This is the common chain
// shape used by cache facades and decorators.
func TestChainedSiblingReturnFunctionalErasureRoundTrip(t *testing.T) {
	t.Parallel()
	const main = "ChainedSiblingFunctionalErasureCall"
	const source = `import java.util.concurrent.CompletableFuture;
import java.util.function.BiFunction;

final class ChainedLocalCache<K, V> {
  V compute(K key, BiFunction<? super K, ? super V, ? extends V> remap,
      boolean recordStats, boolean recordLoad, boolean notifyWriter) {
    return remap.apply(key, null);
  }
}

final class ChainedCacheOwner<K, V> {
  private final ChainedLocalCache<K, CompletableFuture<V>> cache = new ChainedLocalCache<>();
  ChainedLocalCache<K, CompletableFuture<V>> cache() { return cache; }
}

public class ChainedSiblingFunctionalErasureCall<K, V> {
  private final ChainedCacheOwner<K, V> owner = new ChainedCacheOwner<>();

  @SuppressWarnings({"rawtypes", "unchecked"})
  CompletableFuture<V> update(K key, V next) {
    BiFunction<Object, CompletableFuture, CompletableFuture> remap =
        (ignored, oldValue) -> oldValue == null ? CompletableFuture.completedFuture(next) : oldValue;
    return (CompletableFuture<V>) owner.cache().compute(key, (BiFunction) remap, false, false, false);
  }

  public static void main(String[] args) {
    System.out.print(new ChainedSiblingFunctionalErasureCall<String, Integer>().update("answer", 42).join());
  }
}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"-g", "-g:none"} {
		originalDir := t.TempDir()
		sourcePath := filepath.Join(originalDir, main+".java")
		if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", originalDir, sourcePath)
		if out, err := compile.CombinedOutput(); err != nil {
			t.Fatalf("compile %s %s: %v\n%s", main, debug, err, out)
		}
		want := t04RunJava(t, java, originalDir, main)
		classBytes, err := os.ReadFile(filepath.Join(originalDir, main+".class"))
		if err != nil {
			t.Fatal(err)
		}
		resolver := func(internalName string) ([]byte, bool) {
			data, readErr := os.ReadFile(filepath.Join(originalDir, filepath.FromSlash(internalName)+".class"))
			return data, readErr == nil
		}
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{
				Mode: mode, TargetSourceVersion: 8, Resolve: resolver,
			})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, ".cache().compute(") || !strings.Contains(result.Source, "(BiFunction)(") {
				t.Fatalf("missing chained-receiver raw bridge in %s/%s:\n%s", mode, debug, result.Source)
			}

			rebuiltDir := t.TempDir()
			rebuiltSource := filepath.Join(rebuiltDir, main+".java")
			if err := os.WriteFile(rebuiltSource, []byte(result.Source), 0o644); err != nil {
				t.Fatal(err)
			}
			rebuild := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8",
				"-cp", originalDir, "-d", rebuiltDir, rebuiltSource)
			if out, err := rebuild.CombinedOutput(); err != nil {
				t.Fatalf("rebuild %s/%s: %v\n%s\nsource:\n%s", mode, debug, err, out, result.Source)
			}
			classpath := rebuiltDir + string(os.PathListSeparator) + originalDir
			if got := t04RunJava(t, java, classpath, main); got != want {
				t.Fatalf("runtime mismatch %s/%s: got %q want %q", mode, debug, got, want)
			}

			off, err := DecompileWithOptions(classBytes, DecompileOptions{
				Mode: mode, TargetSourceVersion: 8, Resolve: resolver,
				EnvSnapshot: map[string]string{"JDEC_GENERIC_PARAM_RECV_METHOD_OFF": "1"},
			})
			if err != nil {
				t.Fatalf("decompile kill switch %s/%s: %v", mode, debug, err)
			}
			if strings.Contains(off.Source, "(BiFunction)(") {
				t.Fatalf("kill switch retained chained-receiver bridge in %s/%s:\n%s", mode, debug, off.Source)
			}
		}
	}
}

// A lower-bounded wildcard accepts values of its bound, but that bound is lost
// from the invoke descriptor. Source casts to T erase to Object, so the call
// renderer has to recover T from Consumer/Function/BiFunction's receiver type.
func TestLowerBoundedFunctionalArgumentsRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Parallel()
	const main = "LowerBoundedFunctionalCall"
	const source = `import java.util.function.BiFunction;
import java.util.function.Consumer;
import java.util.function.Function;

public class LowerBoundedFunctionalCall<T> {
  @SuppressWarnings("unchecked")
  public String invoke(Object raw, Consumer<? super T> sink,
      Function<? super T, String> one,
      BiFunction<? super T, ? super T, String> two) {
    sink.accept((T) raw);
    return one.apply((T) raw) + ":" + two.apply((T) raw, (T) raw);
  }
  public static void main(String[] args) {
    StringBuilder seen = new StringBuilder();
    LowerBoundedFunctionalCall<String> call = new LowerBoundedFunctionalCall<>();
    String result = call.invoke("x", seen::append, String::toUpperCase, (a, b) -> a + b);
    System.out.print(seen + ":" + result);
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			for _, call := range []string{".accept(", ".apply("} {
				if !strings.Contains(result.Source, call) {
					t.Fatalf("fixture lost %s in %s/%s:\n%s", call, mode, debug, result.Source)
				}
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("lower-bounded functional arguments %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// javac lowers a capturing lambda to a synthetic method whose descriptor carries
// only erased capture types. The invokedynamic call site still knows that the
// captured functional values are lower-bounded consumers. The decompiler must
// project those call-site types back onto the synthetic method's leading capture
// parameters before rendering its body, or the apply/accept arguments remain
// Object and javac rejects the reconstructed source with a CAP# error.
func TestCapturedLowerBoundedFunctionalArgumentsRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Parallel()
	const main = "CapturedLowerBoundedFunctionalCall"
	const source = `import java.util.function.BiFunction;
import java.util.function.Consumer;
import java.util.function.Function;
import java.util.function.Supplier;

public class CapturedLowerBoundedFunctionalCall<T> {
  @SuppressWarnings("unchecked")
  public Supplier<String> defer(Object raw, Consumer<? super T> sink,
      Function<? super T, String> one,
      BiFunction<? super T, ? super T, String> two) {
    return () -> {
      sink.accept((T) raw);
      return one.apply((T) raw) + ":" + two.apply((T) raw, (T) raw);
    };
  }
  public static void main(String[] args) {
    StringBuilder seen = new StringBuilder();
    CapturedLowerBoundedFunctionalCall<String> call = new CapturedLowerBoundedFunctionalCall<>();
    String result = call.defer("x", seen::append, String::toUpperCase, (a, b) -> a + b).get();
    System.out.print(seen + ":" + result);
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			for _, call := range []string{".accept(", ".apply("} {
				if !strings.Contains(result.Source, call) {
					t.Fatalf("capturing fixture lost %s in %s/%s:\n%s", call, mode, debug, result.Source)
				}
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("captured lower-bounded functional arguments %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// A lower bound can itself be parameterized. Its raw class survives as a
// checkcast in bytecode, while its type arguments do not. Passing that raw value
// to a captured `? super List<T>` still needs the source-level `(List<T>)` cast;
// matching erasures alone are not sufficient under wildcard capture conversion.
func TestCapturedParameterizedLowerBoundRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "CapturedParameterizedLowerBound"
	const source = `import java.util.Arrays;
import java.util.List;
import java.util.function.Function;
import java.util.function.Supplier;

public class CapturedParameterizedLowerBound<T> {
  @SuppressWarnings("unchecked")
  public Supplier<String> defer(Object raw, Function<? super List<T>, String> fn) {
    return () -> fn.apply((List<T>) raw);
  }
  public static void main(String[] args) {
    CapturedParameterizedLowerBound<String> call = new CapturedParameterizedLowerBound<>();
    System.out.print(call.defer(Arrays.asList("x"), values -> values.get(0)).get());
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, ".apply(") {
				t.Fatalf("parameterized lower-bound fixture lost apply in %s/%s:\n%s", mode, debug, result.Source)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("captured parameterized lower bound %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// The body is decoded with raw Entry, whereas compute's Signature accepts
// ? super Entry<K,V>. Preserving that legal input supertype is necessary for
// writes through erased captures. Exercise both branches and null results so
// return bridges and side effects are checked against the original bytecode.
func TestT19ContravariantLambdaErasedReceiverRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Parallel()
	const main = "ContravariantLambdaBody"
	const source = `import java.util.*;
import java.util.function.*;
public class ContravariantLambdaBody<K,V> {
  private final Map<Object, Map.Entry<K,V>> entries = new HashMap<>();
  V change(Object key, V value, boolean remove) {
    Object[] box = new Object[]{value};
    BiFunction<Object, Map.Entry<K,V>, Map.Entry<K,V>> update = (k, entry) -> {
      if (remove) { return null; }
      if (entry == null) { return null; }
      entry.setValue((V) box[0]);
      return entry;
    };
    Map.Entry<K,V> result = entries.compute(key, update);
    return result == null ? null : result.getValue();
  }
  public static void main(String[] args) {
    ContravariantLambdaBody<String,Integer> c = new ContravariantLambdaBody<>();
    c.entries.put("a", new AbstractMap.SimpleEntry<>("a",1));
    System.out.print(c.change("a",7,false)+":"+c.change("b",9,false)+":"+c.change("a",8,true)+":"+c.entries.size());
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatal(err)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("erased receiver %s/%s: %v\n%s", mode, debug, err, result.Source)
			}
		}
	}
}

// A class's <T> does not prove Function's input is T. The former textual
// rewrite chose the first enclosing type variable and could change overloads
// or make a correctly typed assignment uncompilable. Also retain the old
// consumer-capture example as an executable oracle, rather than checking a
// particular rewritten cast spelling.
func TestT19PolyCastsKeepDeclaredInputsRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	t.Parallel()
	const main = "PolyCastBoundaries"
	const source = `import java.util.*;
import java.util.function.*;
import java.util.concurrent.*;
public class PolyCastBoundaries<T> {
  String evaluate(boolean flag) {
    Function<Object,CompletionStage> f = null;
    if (flag) { f = x -> CompletableFuture.completedFuture(x); }
    else { f = x -> CompletableFuture.completedFuture("other"); }
    return String.valueOf(f.apply("text").toCompletableFuture().join());
  }
  static <U> long visit(Collection<U> values, Consumer<U> out) {
    return values.stream().map(x -> { out.accept(x); return x; }).filter(x -> x != null).count();
  }
  public static void main(String[] args) {
    PolyCastBoundaries<Integer> p = new PolyCastBoundaries<>();
    StringBuilder result = new StringBuilder();
    System.out.print(p.evaluate(true)+":"+p.evaluate(false)+":"+
      visit(Arrays.asList(3,4), x -> result.append(x))+":"+result);
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatal(err)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("poly cast boundary %s/%s: %v\n%s", mode, debug, err, result.Source)
			}
		}
	}
}
