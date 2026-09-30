package javaclassparser

import "testing"

// Constructor fields must receive the clone from the guarded branch, not a
// null-initialized temporary that loses the result at the ternary merge.
func TestConditionalArrayCloneStoreRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "ConditionalArrayCloneStore"
	source := `public final class ConditionalArrayCloneStore {
  private final Class<?>[] types;
  private final Object[] arguments;
  public ConditionalArrayCloneStore(Class<?>[] types, Object[] arguments) {
    this.types = types != null ? types.clone() : null;
    this.arguments = arguments != null ? arguments.clone() : null;
  }
  public static void main(String[] args) {
    Class<?>[] types = new Class<?>[]{String.class};
    Object[] values = new Object[]{"value"};
    ConditionalArrayCloneStore copy = new ConditionalArrayCloneStore(types, values);
    System.out.print(copy.types.length + ":" + copy.arguments.length + ":" +
      (copy.types != types) + ":" + (copy.arguments != values) + ":" +
      (new ConditionalArrayCloneStore(null, null).types == null));
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := rebuild("8", main, result.Source, want); err != nil {
			t.Fatalf("conditional array clone %s: %v\n%s", mode, err, result.Source)
		}
	}
}
