package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A stateless method reference passed as the final call argument is a Java
// poly expression: its target FI signature supplies generic parameter types.
// A synthetic raw local loses that target and turns valid source into an
// invalid method reference. The runtime oracle also checks earlier arguments
// still run before the reference is constructed and invoked.
func TestMethodReferenceKeepsPolyTargetTypeRoundTrip(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "MethodRefPolyMain", map[string]string{
		"MethodRefPolyMain.java": `@FunctionalInterface
interface TriFunction<A, B, C, R> { R apply(A a, B b, C c); }
final class Strategy { String id() { return "S"; } }
public class MethodRefPolyMain {
  static int events;
  static String mark(int digit, String value) {
    events = events * 10 + digit;
    return value;
  }
  private static <T> String less(Comparable<? super T> a, T b, Strategy s) {
    return a.compareTo(b) + ":" + s.id();
  }
  private static void use(String a, String b,
      TriFunction<Comparable<? super String>, String, Strategy, String> fn) {
    System.out.println(fn.apply(a, b, new Strategy()));
  }
  public static void main(String[] args) {
    use(mark(1, "a"), mark(2, "b"), MethodRefPolyMain::less);
    System.out.println(events);
  }
}`,
	})
	if want := "-1:S\n12\n"; original != want {
		t.Fatalf("independent Java oracle changed: got %q, want %q", original, want)
	}

	javac, java := t04Tools(t)
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			resolver := func(internalName string) ([]byte, bool) {
				base := internalName
				if i := strings.LastIndexByte(internalName, '/'); i >= 0 {
					base = internalName[i+1:]
				}
				b, ok := classes[base]
				return b, ok
			}
			sourceDir, outDir := t.TempDir(), t.TempDir()
			var files []string
			var combined strings.Builder
			for name, raw := range classes {
				result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolver})
				if err != nil {
					t.Fatalf("decompile %s: %v", name, err)
				}
				if result.Source == "" {
					t.Fatalf("decompile %s returned empty source (status=%s)", name, result.Status)
				}
				combined.WriteString(result.Source)
				combined.WriteByte('\n')
				path := filepath.Join(sourceDir, name+".java")
				if err := os.WriteFile(path, []byte(result.Source), 0o644); err != nil {
					t.Fatal(err)
				}
				files = append(files, path)
			}
			args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", "17", "-d", outDir}, files...)
			if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
				t.Fatalf("recompile %s source: %v\n%s\n----- source -----\n%s", mode, err, out, combined.String())
			}
			if !strings.Contains(combined.String(), "MethodRefPolyMain::less") {
				t.Fatalf("decompiler did not retain the method reference:\n%s", combined.String())
			}
			if strings.Contains(combined.String(), "TriFunction var") {
				t.Fatalf("method reference was erased into a raw synthetic local:\n%s", combined.String())
			}
			if got := t04RunJava(t, java, outDir, "MethodRefPolyMain"); got != original {
				t.Fatalf("%s runtime mismatch: got %q want %q", mode, got, original)
			}
		})
	}
}
