package javaclassparser

import (
	"strings"
	"testing"
)

func TestGenericNullArgumentUsesRecoveredFormal(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "GenericNullMain", map[string]string{
		"GenericBag.java":  `public interface GenericBag<E> { void add(E value); }`,
		"GenericImpl.java": `public class GenericImpl implements GenericBag<String> { public void add(String value) { System.out.println(value == null ? "null" : value); } }`,
		"GenericNullMain.java": `public class GenericNullMain {
  static void fill(GenericBag<String> values) { values.add(null); }
  public static void main(String[] args) { fill(new GenericImpl()); }
}`,
	})
	if strings.TrimSpace(original) != "null" {
		t.Fatalf("bad fixture oracle: %q", original)
	}
	t04RoundTripModes(t, "17", "GenericNullMain", original, classes, func(t *testing.T, source string) {
		if strings.Contains(source, "add((Object)(null))") || strings.Contains(source, "add((Object)null)") {
			t.Fatalf("generic null argument was pinned to erased Object:\n%s", source)
		}
	})
}

func TestJDKListNullArgumentUsesElementFormal(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "ListNullMain", map[string]string{
		"ListNullMain.java": `import java.util.ArrayList;
import java.util.List;
public class ListNullMain {
  final List<String> values = new ArrayList<String>();
  void fill() { values.add(null); values.add(0, null); values.set(0, null); }
  public static void main(String[] args) { ListNullMain x = new ListNullMain(); x.fill(); System.out.println(x.values.size()); }
}`,
	})
	if strings.TrimSpace(original) != "2" {
		t.Fatalf("bad fixture oracle: %q", original)
	}
	t04RoundTripModes(t, "17", "ListNullMain", original, classes, func(t *testing.T, source string) {
		if strings.Contains(source, "(Object)(null)") || strings.Contains(source, "(Object)null") {
			t.Fatalf("List<E> null argument was pinned to erased Object:\n%s", source)
		}
	})
}
