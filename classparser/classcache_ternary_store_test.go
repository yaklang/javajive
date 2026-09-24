package javaclassparser

import (
	"strings"
	"testing"
)

// A javac-style class$ cache idiom leaves the resolved class on the operand
// stack while DUP/PUTSTATIC stores that same value. When the value is used as a
// call argument, losing the adjacent store turns it into an Object placeholder
// and emits source that no longer type-checks. The original/rebuilt JVM output
// is the independent oracle for both decompiler modes.
func TestClassCacheTernaryStoreRoundTrip(t *testing.T) {
	const main = "ClassCacheTernaryStore"
	source := `public final class ClassCacheTernaryStore {
  private static Class class$Foo;
  private static Class class$(String name) {
    try { return Class.forName(name); }
    catch (ClassNotFoundException e) { return null; }
  }
  private static Class lookup(Class<?> type) { return type; }
  private static Class cached() {
    return lookup(class$Foo == null ? (class$Foo = class$("java.lang.String")) : class$Foo);
  }
  public static void main(String[] args) { System.out.println(cached().getName()); }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	if strings.TrimSpace(want) != "java.lang.String" {
		t.Fatalf("independent javac/java oracle changed: %q", want)
	}
	raw := classes[main]
	if len(raw) == 0 {
		t.Fatal("javac did not produce class-cache fixture")
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
			t.Fatalf("class-cache round-trip %s (status %s, diagnostics %+v): %v\n%s", mode, result.Status, result.Diagnostics, err, result.Source)
		}
	}
}

func TestMapCacheTernaryLocalRoundTrip(t *testing.T) {
	const main = "MapCacheTernaryLocal"
	source := `import java.util.HashMap;
import java.util.Map;
public final class MapCacheTernaryLocal {
  private static final Map<String, String> cache = new HashMap<>();
  private static String lookup(Class<?> type) {
    String cached = type != null ? (String)cache.get(type.getName()) : null;
    return cached != null ? cached : "missing";
  }
  public static void main(String[] args) {
    cache.put("java.lang.String", "hit");
    System.out.println(lookup(String.class) + ":" + lookup(null));
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	if strings.TrimSpace(want) != "hit:missing" {
		t.Fatalf("independent javac/java oracle changed: %q", want)
	}
	raw := classes[main]
	if len(raw) == 0 {
		t.Fatal("javac did not produce map-cache fixture")
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
			t.Fatalf("map-cache ternary round-trip %s (status %s, diagnostics %+v): %v\n%s", mode, result.Status, result.Diagnostics, err, result.Source)
		}
	}
}
