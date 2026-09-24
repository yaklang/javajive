package javaclassparser

import (
	"strings"
	"testing"
)

// javac keeps array-initializer operands live for a delegating constructor
// call. Lowering may materialize them as locals, but Java requires this(...)
// to remain the first statement. The independent runtime oracle makes sure
// the rewrite preserves the array values and constructor effects.
func TestCtorArrayThisSpillsRoundTrip(t *testing.T) {
	const main = "CtorArrayThisSpill"
	source := `public final class CtorArrayThisSpill {
  private final String value;
  private CtorArrayThisSpill(char[] spaces, char[] newline) {
    this(new String(spaces) + ":" + (int)newline[0]);
  }
  private CtorArrayThisSpill(String value) { this.value = value; }
  private CtorArrayThisSpill() { this(new char[]{32, 32}, new char[]{10}); }
  public static void main(String[] args) { System.out.println(new CtorArrayThisSpill().value); }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	if strings.TrimSuffix(want, "\n") != "  :10" {
		t.Fatalf("independent javac/java oracle changed: %q", want)
	}
	raw := classes[main]
	if len(raw) == 0 {
		t.Fatal("javac did not produce constructor-spill fixture")
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
			t.Fatalf("constructor-array-spill round-trip %s (status %s, diagnostics %+v): %v\n%s", mode, result.Status, result.Diagnostics, err, result.Source)
		}
	}
}

// The constructor's earlier arguments have observable effects before javac's
// synthetic array temp. Inlining the array is safe only if GETSTATIC/call/new
// producers prove they completed before the array allocation; the independent
// runtime trace checks their original left-to-right order.
func TestCtorArrayThisSpillPreservesEarlierArgumentOrder(t *testing.T) {
	const main = "CtorArrayThisSpillOrder"
	source := `public final class CtorArrayThisSpillOrder {
  private static final StringBuilder EVENTS = new StringBuilder();
  private final String value;
  private static String prefix() { EVENTS.append("P"); return "prefix"; }
  private static String marker() { EVENTS.append("M"); return "marker"; }
  private static String element() { EVENTS.append("A"); return "element"; }
  private CtorArrayThisSpillOrder(String first, StringBuilder marker, String[] values) {
    this.value = first + ":" + marker.toString() + ":" + values[0];
  }
  private CtorArrayThisSpillOrder() {
    this(prefix(), new StringBuilder(marker()), new String[]{element()});
  }
  public static void main(String[] args) {
    CtorArrayThisSpillOrder value = new CtorArrayThisSpillOrder();
    System.out.println(EVENTS + ":" + value.value);
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	if strings.TrimSpace(want) != "PMA:prefix:marker:element" {
		t.Fatalf("independent javac/java oracle changed: %q", want)
	}
	raw := classes[main]
	if len(raw) == 0 {
		t.Fatal("javac did not produce constructor-order fixture")
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
			t.Fatalf("constructor-array order round-trip %s (status %s, diagnostics %+v): %v\n%s", mode, result.Status, result.Diagnostics, err, result.Source)
		}
	}
}
