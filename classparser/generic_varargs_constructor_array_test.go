package javaclassparser

import "testing"

// A generic array returned by copy remains the complete array argument to a
// varargs constructor. Casting it to the element type makes the source invalid.
func TestGenericVarargsConstructorArrayRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "GenericVarargsConstructorArray"
	source := `import java.util.function.Predicate;
public final class GenericVarargsConstructorArray<T> implements Predicate<T> {
  private final Predicate<? super T>[] predicates;
  @SafeVarargs public GenericVarargsConstructorArray(Predicate<? super T>... values) {
    this.predicates = values;
  }
  @SafeVarargs private static <T> Predicate<T>[] copy(Predicate<? super T>... values) {
    return (Predicate<T>[])values.clone();
  }
  @SafeVarargs public static <T> Predicate<T> all(Predicate<? super T>... values) {
    if (values.length == 0) return new GenericVarargsConstructorArray<T>();
    if (values.length == 1) return (Predicate<T>)values[0];
    return new GenericVarargsConstructorArray<T>(copy(values));
  }
  public boolean test(T value) {
    for (Predicate<? super T> predicate : predicates)
      if (!predicate.test(value)) return false;
    return true;
  }
  public static void main(String[] args) {
    Predicate<Integer> positive = new GenericVarargsConstructorArray<Integer>();
    System.out.print(all(positive, positive).test(7) + ":" + all(positive).test(7));
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := rebuild("8", main, result.Source, want); err != nil {
			t.Fatalf("varargs array argument %s: %v\n%s", mode, err, result.Source)
		}
	}
}

// The leading boolean shifts every formal of the delegated constructor.
// Different arities cannot select the enclosing constructor, so no overload
// pin cast may turn the third array argument into a single Predicate.
func TestShiftedThisCtorGenericArrayRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "ShiftedThisCtorGenericArray"
	source := `import java.util.function.Predicate;
public final class ShiftedThisCtorGenericArray<T> {
  private final Predicate<? super T>[] predicates;
  public ShiftedThisCtorGenericArray(Predicate<? super T>[] first,
      Predicate<? super T>[] second, Predicate<? super T> fallback) {
    this(true, first, second, fallback);
  }
  private ShiftedThisCtorGenericArray(boolean copy, Predicate<? super T>[] first,
      Predicate<? super T>[] second, Predicate<? super T> fallback) {
    this.predicates = second;
  }
  public int count() { return predicates.length; }
  public static void main(String[] args) {
    Predicate<Integer>[] empty = new Predicate[0];
    System.out.print(new ShiftedThisCtorGenericArray<Integer>(empty, empty, null).count());
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := rebuild("8", main, result.Source, want); err != nil {
			t.Fatalf("shifted this(...) array %s: %v\n%s", mode, err, result.Source)
		}
	}
}
