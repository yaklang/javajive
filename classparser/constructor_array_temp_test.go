package javaclassparser

import (
	"strings"
	"testing"
)

// The array-fill lowering can materialize an initialized array as a local
// before the invokespecial that implements `this(...)`. That local may be
// nested in another `new` expression inside the delegated constructor's
// argument list (not only passed as a direct argument). Keep the source legal
// and preserve left-to-right evaluation of the array element and later args.
func TestCtorDelegationNestedArrayTempRoundTrip(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "NestedArrayDelegationMain", map[string]string{
		"NestedArrayDelegationMain.java": `public class NestedArrayDelegationMain {
  static int sequence;
  final int value;

  NestedArrayDelegationMain(int input) {
    this(new Box(new Object[]{mark(input)}), mark(input + 1));
  }

  NestedArrayDelegationMain(Box box, int next) {
    value = ((Integer) box.items[0]) + next;
  }

  NestedArrayDelegationMain(int input, boolean reverse) {
    this(mark(input + 1), new Box(new Object[]{mark(input)}));
  }

  NestedArrayDelegationMain(int next, Box box) {
    value = next + ((Integer) box.items[0]);
  }

  static int mark(int value) {
    sequence = sequence * 10 + value;
    return value;
  }

  static final class Box {
    final Object[] items;
    Box(Object[] items) { this.items = items; }
  }

  public static void main(String[] args) {
    NestedArrayDelegationMain result = new NestedArrayDelegationMain(1);
    String first = result.value + ":" + sequence;
    sequence = 0;
    NestedArrayDelegationMain reversed = new NestedArrayDelegationMain(1, true);
    System.out.println(first + "/" + reversed.value + ":" + sequence);
  }
}`,
	})
	if strings.TrimSpace(original) != "3:12/3:21" {
		t.Fatalf("fixture oracle changed: %q, want 3:12/3:21", original)
	}
	t04RoundTripModes(t, "17", "NestedArrayDelegationMain", original, classes, func(t *testing.T, source string) {
		if !strings.Contains(source, "this(") {
			t.Fatalf("round-tripped source lost constructor delegation:\n%s", source)
		}
	})
}
