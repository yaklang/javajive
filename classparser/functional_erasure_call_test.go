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
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
				t.Fatalf("materialized BiFunction %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// A class generic method carries the same erased SAM problem even when the
// argument is a local and the callee is declared by the current class.
func TestSameClassBiFunctionTypeVariablesRoundTrip(t *testing.T) {
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
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
				t.Fatalf("same-class BiFunction %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// Function<T,R> loses T in the instantiated SAM descriptor, while
// Supplier<Iterator<Entry<T,R>>> loses the nested Iterator arguments. Both
// shapes appear in Caffeine's same-class helper calls.
func TestSameClassFunctionAndSupplierErasureRoundTrip(t *testing.T) {
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
	for _, debug := range []string{"-g", "-g:none"} {
		want, classBytes := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatalf("decompile %s/%s: %v", mode, debug, err)
			}
			if !strings.Contains(result.Source, "installMapper(") || !strings.Contains(result.Source, "installSupplier(") {
				t.Fatalf("fixture lost generic helper calls in %s/%s:\n%s", mode, debug, result.Source)
			}
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
				t.Fatalf("Function/Supplier erasure %s/%s: %v", mode, debug, err)
			}
		}
	}
}

// A lower-bounded wildcard accepts values of its bound, but that bound is lost
// from the invoke descriptor. Source casts to T erase to Object, so the call
// renderer has to recover T from Consumer/Function/BiFunction's receiver type.
func TestLowerBoundedFunctionalArgumentsRoundTrip(t *testing.T) {
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
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
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
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
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
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
				t.Fatalf("captured parameterized lower bound %s/%s: %v", mode, debug, err)
			}
		}
	}
}
