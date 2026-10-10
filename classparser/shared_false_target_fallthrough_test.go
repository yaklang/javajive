package javaclassparser

import "testing"

// Both failed predicates in the compound guard enter the same suffix. The
// suffix must run even when the first predicate's index == 0 branch was taken.
func TestSharedFalseTargetFallthroughRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "SharedFalseTargetFallthrough"
	source := `public final class SharedFalseTargetFallthrough {
  private static boolean contains(String text, int start, int len, String... choices) {
    if (start < 0 || start + len > text.length()) return false;
    String part = text.substring(start, start + len);
    for (String choice : choices) if (part.equals(choice)) return true;
    return false;
  }
  private int advance(String text, int index) {
    if (contains(text,index-1,3,"ISL","YSL")) {
      index++;
    } else if (index == 0 && contains(text,index,5,"SUGAR")) {
      index++;
    } else if (contains(text,index,2,"SH")) {
      index += 2;
    } else if ((index == 0 && contains(text,index+1,1,"M","N","L","W"))
        || contains(text,index+1,1,"Z")) {
      index += contains(text,index+1,1,"Z") ? 2 : 1;
    } else if (contains(text,index,2,"SC")) {
      index += 3;
    } else {
      index++;
    }
    return index;
  }
  public static void main(String[] args) {
    SharedFalseTargetFallthrough probe = new SharedFalseTargetFallthrough();
    for (String value : new String[]{"SCHCH","SHADOW","SUGAR","SIMPLE","SMITH","SNOW","SLIM","SWAN","SZ","SZA","SZZ","SZI","ISLAND","YSLAND"})
      System.out.print(value + ":" + probe.advance(value,0) + ",");
    System.out.print(probe.advance("XSHADOW",1));
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := rebuild("8", main, result.Source, want); err != nil {
			t.Fatalf("shared false-target fallthrough %s: %v\n%s", mode, err, result.Source)
		}
	}
}
