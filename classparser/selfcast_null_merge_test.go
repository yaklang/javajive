package javaclassparser

import (
	"strings"
	"testing"
)

// The selected value remains live across the side effects after the merge.
// Inlining the cast into its own conditional arm must preserve both the value
// and the empty-list null path in each decompiler mode.
func TestSelfCastNullMergeRoundTrip(t *testing.T) {
	const main = "SelfCastNullMerge"
	source := `import java.util.ArrayList;
import java.util.List;
public final class SelfCastNullMerge {
  private final List<Object> children = new ArrayList<>();
  private int effects;
  private SelfCastNullMerge unwrap() {
    SelfCastNullMerge selected = children.size() > 0 ? (SelfCastNullMerge)children.get(0) : null;
    effects++;
    return selected;
  }
  public static void main(String[] args) {
    SelfCastNullMerge root = new SelfCastNullMerge();
    System.out.print(root.unwrap() == null ? "empty" : "wrong");
    SelfCastNullMerge child = new SelfCastNullMerge();
    root.children.add(child);
    System.out.print(":" + (root.unwrap() == child ? "child" : "wrong"));
    System.out.println(":" + root.effects);
  }
}`
	want, classes := t17CompileRun(t, "8", main, map[string]string{main + ".java": source})
	if strings.TrimSpace(want) != "empty:child:2" {
		t.Fatalf("independent javac/java oracle changed: %q", want)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		result, err := DecompileWithOptions(classes[main], DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("decompile %s: %v", mode, err)
		}
		if err := t17RebuildRunErr(t, "8", main, result.Source, want); err != nil {
			t.Fatalf("self-cast null-merge round-trip %s: %v\n%s", mode, err, result.Source)
		}
	}
}
