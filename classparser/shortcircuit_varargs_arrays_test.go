package javaclassparser

import (
	"strings"
	"testing"
)

// Every short-circuit arm constructs its own String[] before the varargs call.
// The source is checked against the original JVM result, so an Object/null
// placeholder cannot satisfy the Java compiler or silently change a branch.
func TestShortCircuitVarargsArraysRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "ShortCircuitVarargsArrays"
	source := `public final class ShortCircuitVarargsArrays {
  private static final String[] END = {"L", "R", "N", "M", "B", "H", "F", "V", "W", " "};
  private static boolean contains(String text, int start, int len, String... choices) {
    if (start < 0 || start + len > text.length()) return false;
    String part = text.substring(start, start + len);
    for (String choice : choices) if (part.equals(choice)) return true;
    return false;
  }
  private boolean conditionCH1(String text, int index) {
    return contains(text,0,4,"VAN ","VON ")
        || contains(text,0,3,"SCH")
        || contains(text,index-2,6,"ORCHES","ARCHIT","ORCHID")
        || contains(text,index+2,1,"T","S")
        || ((index == 0 || !contains(text,index-1,1,"A","O","U","E"))
            && (contains(text,index+2,1,END) || index+1 == text.length()-1));
  }
  private boolean retainedArray(String text, int index) {
    if (text.charAt(index+1) == 'M') return true;
    String[] first = {"UMB"};
    String[] second = {"ARRAY_ONCE_SENTINEL"};
    return contains(text,index-1,3,first)
        && ((index+1 == text.length()-1) || contains(text,index+2,2,second));
  }
  private boolean conditionC0(String text, int index) {
    if (contains(text,index,4,"CHIA")) return true;
    if (index <= 1) return false;
    if ("AEIOUY".indexOf(text.charAt(index-2)) >= 0) return false;
    if (!contains(text,index-1,3,"ACH")) return false;
    char next = text.charAt(index+2);
    return (next != 'I' && next != 'E')
        || contains(text,index-2,6,"BACHER","MACHER");
  }
  public static void main(String[] args) {
    ShortCircuitVarargsArrays probe = new ShortCircuitVarargsArrays();
    for (String value : new String[]{"VAN CH", "SCHCH", "ORCHES", "ARCHIT", "CHT", "CHS", "CH", "ACH"})
      System.out.print((probe.conditionCH1(value, 0) ? "1" : "0") + ",");
    System.out.print(probe.retainedArray("UMBXYZ", 1));
    for (String value : new String[]{"BACHER","MACHER","XACHER","XACHOR"})
      System.out.print(probe.conditionC0(value, 2) ? "1" : "0");
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := rebuild("8", main, result.Source, want); err != nil {
			t.Fatalf("short-circuit varargs array round-trip %s: %v\n%s", mode, err, result.Source)
		}
		// A retained local initializer must not be repeated at the later call:
		// duplicate array allocation can change the first observable OOME point.
		if n := strings.Count(result.Source, "ARRAY_ONCE_SENTINEL"); n != 1 {
			t.Fatalf("retained array initializer rendered %d times in %s", n, mode)
		}
		// One occurrence belongs to main's input and one to conditionC0's
		// branch-local array. A second initializer would change allocation count.
		if n := strings.Count(result.Source, `"MACHER"`); n != 2 {
			t.Fatalf("conditionC0 array element rendered %d times in %s", n, mode)
		}
	}
}
