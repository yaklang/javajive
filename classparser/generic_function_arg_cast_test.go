package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenericFunctionalValueKeepsErasedCallDescriptor covers a captured lambda
// materialized as a local before a generic Map.computeIfAbsent call on a static
// field. Its invokedynamic descriptor preserves only Function<String, List>;
// the receiver signature expects Function<? super String, ? extends
// List<Integer>>. Keeping the call's raw Function descriptor lets javac
// preserve the receiver's key inference without inventing unavailable nested
// lambda return types.
func TestGenericFunctionalValueKeepsErasedCallDescriptor(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			root := t.TempDir()
			originalDir := filepath.Join(root, "original")
			if err := os.MkdirAll(originalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			originalPath := filepath.Join(root, "GenericFunctionArg.java")
			original := `import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class GenericFunctionArg {
  private static final Map<String, List<Integer>> table = new HashMap<>();
  private static void add(String key, int value) {
    table.computeIfAbsent(key, ignored -> {
      ArrayList<Integer> rows = new ArrayList<>();
      rows.add(value);
      return rows;
    }).add(value);
  }
  public static void main(String[] args) {
    add("x", 1);
    add("x", 2);
    add("y", 3);
    System.out.println(table.get("x").get(0) + "," +
        table.get("x").get(1) + "," + table.get("x").get(2) + ";" +
        table.get("y").get(0) + "," + table.get("y").get(1));
  }
}`
			if err := os.WriteFile(originalPath, []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			compileOriginal := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", originalDir, originalPath)
			if out, err := compileOriginal.CombinedOutput(); err != nil {
				t.Fatalf("compile original generic functional fixture: %v\n%s", err, out)
			}
			want := t04RunJava(t, java, originalDir, "GenericFunctionArg")
			classBytes, err := os.ReadFile(filepath.Join(originalDir, "GenericFunctionArg.class"))
			if err != nil {
				t.Fatal(err)
			}

			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode})
					if err != nil {
						t.Fatalf("decompile generic functional fixture: %v", err)
					}
					if !strings.Contains(result.Source, "Function") {
						t.Fatalf("fixture no longer exercises the materialized Function value:\n%s", result.Source)
					}
					rebuiltDir := filepath.Join(root, string(mode), "rebuilt")
					if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
						t.Fatal(err)
					}
					decompiledPath := filepath.Join(rebuiltDir, "GenericFunctionArg.java")
					if err := os.WriteFile(decompiledPath, []byte(result.Source), 0o644); err != nil {
						t.Fatal(err)
					}
					compileRebuilt := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-cp", originalDir, "-d", rebuiltDir, decompiledPath)
					if out, err := compileRebuilt.CombinedOutput(); err != nil {
						t.Fatalf("recompile decompiled generic functional fixture (%s/%s): %v\n%s\n----- source -----\n%s", mode, debug, err, out, result.Source)
					}
					classpath := rebuiltDir + string(os.PathListSeparator) + originalDir
					if got := t04RunJava(t, java, classpath, "GenericFunctionArg"); got != want {
						t.Fatalf("generic functional behavior changed (%s/%s): got %q want %q\n%s", mode, debug, got, want, result.Source)
					}
				})
			}
		})
	}
}
