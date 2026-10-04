package javaclassparser

import "testing"

// A self-bounded generic interface keeps the source-level T formal even when
// the invokeinterface descriptor uses its erased bound. Overloaded numeric
// methods make an Object pin particularly harmful: multiply(Object) does not
// exist, while multiply(T) is the branch selected by the original bytecode.
func TestGenericFieldElementOpsRoundTrip(t *testing.T) {
	const main = "GenericFieldElementOps"
	const elements = `interface BaseElement<T extends BaseElement<T>> {
  T multiply(T other); T multiply(int n);
  T subtract(T other); T add(T other); T divide(T other);
  int value();
}
interface GFElement<T extends GFElement<T>> extends BaseElement<T> {
  T multiply(double n);
}
final class GFScalar implements GFElement<GFScalar> {
  private final int n;
  GFScalar(int n) { this.n = n; }
  public GFScalar multiply(GFScalar other) { return new GFScalar(n * other.n); }
  public GFScalar multiply(int k) { return new GFScalar(n * k); }
  public GFScalar multiply(double k) { return new GFScalar((int)(n * k)); }
  public GFScalar subtract(GFScalar other) { return new GFScalar(n - other.n); }
  public GFScalar add(GFScalar other) { return new GFScalar(n + other.n); }
  public GFScalar divide(GFScalar other) { return new GFScalar(n / other.n); }
  public int value() { return n; }
}`
	const source = `public final class GenericFieldElementOps<T extends GFElement<T>> {
  private final T x;
  public GenericFieldElementOps(T x) { this.x = x; }
  public T calculate(T a, T b) {
    return a.multiply(this.x).subtract(b).add(a).divide(this.x);
  }
  public static void main(String[] args) {
    GenericFieldElementOps<GFScalar> ops = new GenericFieldElementOps<GFScalar>(new GFScalar(2));
    System.out.print(ops.calculate(new GFScalar(8), new GFScalar(2)).value());
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source, "Elements.java": elements})
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8,
			Resolve: func(internalName string) ([]byte, bool) {
				b, ok := classes[internalName]
				return b, ok
			},
		})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		got, _ := t17CompileRun(t, "8", main, map[string]string{main + ".java": result.Source, "Elements.java": elements})
		if got != want {
			t.Fatalf("generic field operation %s: got %q, want %q\n%s", mode, got, want, result.Source)
		}
	}
}
