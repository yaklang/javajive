package javaclassparser

import "testing"

// A single-use COPY does not make its source single-use. The oracle checks
// iteration count, allocation count, mutated contents, identity and a scalar
// snapshot after reassignment, rather than generated variable names.
func TestAdversarialAliasPreservesEvaluationRoundTrip(t *testing.T) {
	const main = "AliasEvaluation"
	const source = `import java.util.*;
public class AliasEvaluation {
  static int calls;
  static Object allocate() { calls++; return new Object(); }
  static String next(Iterator<?> it) {
    Object value=it.next();
    String seen=String.valueOf(value);
    Object copy=value;
    return seen+":"+copy+":"+it.next();
  }
  static Map<String,Integer> build() {
    LinkedHashMap<String,Integer> values=new LinkedHashMap<>();
    values.put("a",1);
    values.put("b",2);
    Map<String,Integer> alias=values;
    return Collections.unmodifiableMap(alias);
  }
  static boolean identity() {
    Object value=allocate();
    Object copy=value;
    return value==copy;
  }
  static int snapshot() {
    int value=3;
    int copy=value;
    value=9;
    return value*10+copy;
  }
  public static void main(String[] args) {
    System.out.print(next(Arrays.asList("a","b","c").iterator())+":"+build()+":"+identity()+":"+calls+":"+snapshot());
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, raw := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatal(err)
			}
			if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
				t.Fatalf("alias %s/%s: %v\n%s", mode, debug, err, result.Source)
			}
		}
	}
}
