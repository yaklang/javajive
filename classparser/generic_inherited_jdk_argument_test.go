package javaclassparser

import (
	"strings"
	"testing"
)

// A non-generic jar interface can fix a JDK generic ancestor's type argument.
// The inherited List<String>.add descriptor is still (Object), while the source
// formal is String even when the immediate receiver declares another add overload.
func TestInheritedJDKGenericArgumentThroughRawInterfaces(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "InheritedJDKMain", map[string]string{
		"FixedStrings.java": `import java.util.List;
public interface FixedStrings extends List<String> {}`,
		"LazyStrings.java": `public interface LazyStrings extends FixedStrings {
  void add(byte[] value);
}`,
		"LazyStringsImpl.java": `import java.util.ArrayList;
public final class LazyStringsImpl extends ArrayList<String> implements LazyStrings {
  public void add(byte[] value) { add(new String(value)); }
}`,
		"InheritedJDKMain.java": `public class InheritedJDKMain {
  static void append(LazyStrings values, Object value) { values.add((String) value); }
  public static void main(String[] args) {
    LazyStrings values = new LazyStringsImpl();
    append(values, "ok");
    System.out.println(values.get(0));
  }
}`,
	})
	if strings.TrimSpace(original) != "ok" {
		t.Fatalf("bad fixture oracle: %q", original)
	}
	t04RoundTripModes(t, "17", "InheritedJDKMain", original, classes, func(t *testing.T, source string) {
		if strings.Contains(source, ".add((Object)") {
			t.Fatalf("inherited List<String>.add was pinned to erased Object:\n%s", source)
		}
	})
}

// A raw generic receiver has no type argument to substitute. A coincidentally
// same-named type variable on the caller must not be treated as that argument.
func TestRawGenericReceiverDoesNotCaptureCallerTypeVariable(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "RawGenericMain", map[string]string{
		"RawBox.java": `public interface RawBox<T> { void put(T value); }`,
		"RawStringBox.java": `public final class RawStringBox implements RawBox<String> {
  public void put(String value) { System.out.println(value); }
}`,
		"RawGenericMain.java": `public class RawGenericMain<T extends Number> {
  @SuppressWarnings("unchecked")
  void send(RawBox box, Object value) { box.put(value); }
  public static void main(String[] args) {
    new RawGenericMain<Integer>().send(new RawStringBox(), "ok");
  }
}`,
	})
	if strings.TrimSpace(original) != "ok" {
		t.Fatalf("bad fixture oracle: %q", original)
	}
	t04RoundTripModes(t, "17", "RawGenericMain", original, classes, func(t *testing.T, source string) {
		if strings.Contains(source, ".put((T)") {
			t.Fatalf("raw receiver captured the caller's unrelated T:\n%s", source)
		}
	})
}
