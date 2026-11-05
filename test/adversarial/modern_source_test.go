package adversarial

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// Java 21's switch expressions, records, sealed hierarchies, pattern switches,
// private interface methods and enum constant bodies are common in current
// libraries but absent from the Java 8 historical corpus. Keep these shapes in
// the required CI-gated evaluation so silent value loss (switch-expression
// yields) or invalid record/interface output cannot return.
const modernShapeSource = `
public class ModernShape {
    sealed interface Node permits Leaf, Branch {}
    record Leaf(int v) implements Node {
        Leaf { if (v < 0) throw new IllegalArgumentException("v"); }
    }
    record Branch(Node l, Node r) implements Node {}

    interface Formula {
        int apply(int x);
        private int helper(int x) { return x + 1; }
        default int twice(int x) { return helper(apply(x)); }
        static int id(int x) { return x; }
    }
    static final class Impl implements Formula {
        public int apply(int x) { return x * 2; }
    }

    static int sum(Node n) {
        return switch (n) {
            case Leaf leaf -> leaf.v();
            case Branch b -> sum(b.l()) + sum(b.r());
            default -> -1;
        };
    }

    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            int k = Math.floorMod(entry + i, 5);
            String s = switch (k) {
                case 0 -> "a";
                case 1 -> { yield "bb"; }
                case 2 -> "ccc";
                default -> "z";
            };
            int n = switch (k) {
                case 0 -> 1;
                case 1 -> 2;
                case 2 -> { int y = k * 3; yield y; }
                default -> -1;
            };
            Node node = ((entry + i) & 1) == 0
                ? new Leaf(Math.abs(entry + i) % 7)
                : new Branch(new Leaf(1), new Leaf(2));
            acc += s.length() * 10 + n + sum(node) + new Impl().twice(k);
        }
        return acc;
    }
}
`

const switchExpressionSource = `
public class SwitchExpressionShape {
    static String text(int x) {
        return switch (x) {
            case 0 -> "zero";
            case 1 -> "one";
            case 2 -> { yield "two"; }
            default -> "other";
        };
    }
    static int nested(int x) {
        return switch (x) {
            case 0 -> switch (x + 1) { case 1 -> 10; default -> 11; };
            case 1 -> { int y = x + 1; y *= 3; yield y; }
            default -> -1;
        };
    }
    static String maybe(int x) {
        return switch (x) {
            case 0 -> null;
            case 1 -> "one";
            default -> "d";
        };
    }
    static int guarded(int x) {
        try {
            return switch (x) {
                case 0 -> 10;
                case 1 -> { if (x == 1) throw new IllegalStateException(); yield 20; }
                default -> 30;
            };
        } catch (IllegalStateException e) {
            return -1;
        }
    }
    static String ternaryArm(boolean b, int x) {
        return switch (x) {
            case 0 -> b ? "a" : "b";
            case 1 -> b ? "c" : "d";
            default -> b ? "e" : "f";
        };
    }
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            int k = Math.floorMod(entry + i, 4);
            String m = maybe(k);
            boolean b = ((entry + i) & 1) == 0;
            acc = acc * 3 + text(k).length() + nested(k) + (m == null ? 7 : m.length()) + guarded(k) + ternaryArm(b, k).length();
        }
        return acc;
    }
}
`

func javacMajor(t *testing.T, javac string) int {
	t.Helper()
	out, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("javac -version: %v\n%s", err, out)
	}
	for _, field := range strings.Fields(string(out)) {
		version := strings.TrimPrefix(field, "javac")
		if dot := strings.IndexByte(version, '.'); dot >= 0 {
			version = version[:dot]
		}
		if major, err := strconv.Atoi(version); err == nil {
			return major
		}
	}
	t.Fatalf("cannot parse javac version from %q", out)
	return 0
}

func TestAdversarialModernSourceShapes(t *testing.T) {
	if !envEnabled() {
		t.Skip("set JAVAJIVE_ADVERSARIAL_EVAL=1 to run the adversarial evaluation")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	if javacMajor(t, javac) < 21 {
		t.Skip("Java 21 source shapes require javac 21")
	}
	root := t.TempDir()
	driverRoot := filepath.Join(root, "driver")
	if err := os.MkdirAll(driverRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	buildDriver(t, javac, driverRoot)

	const nullPatternSource = `
public class NullPatternShape {
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            Object o = ((entry + i) % 3 == 0) ? null
                : (((entry + i) % 3 == 1) ? ("s" + i) : Integer.valueOf(i));
            String s = switch (o) {
                case null -> "NULL";
                case String x -> "S:" + x;
                default -> "DEFAULT";
            };
            acc = acc * 3 + s.length();
        }
        return acc;
    }
}
`
	for _, eval := range []sourceCase{
		{"ModernShape", map[string]string{"ModernShape.java": modernShapeSource}},
		{"SwitchExpressionShape", map[string]string{"SwitchExpressionShape.java": switchExpressionSource}},
		{"NullPatternShape", map[string]string{"NullPatternShape.java": nullPatternSource}},
	} {
		eval := eval
		t.Run(eval.name, func(t *testing.T) {
			origDir := filepath.Join(root, eval.name+"-orig")
			if err := os.MkdirAll(origDir, 0o755); err != nil {
				t.Fatal(err)
			}
			var sourcePaths []string
			for name, source := range eval.sources {
				path := filepath.Join(origDir, name)
				if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				sourcePaths = append(sourcePaths, path)
			}
			if err := compileTreeAt(t, javac, origDir, sourcePaths, 21); err != nil {
				t.Fatalf("compile original: %v", err)
			}
			original, err := runMatrix(t, java, driverRoot, origDir, eval.name)
			if err != nil {
				t.Fatalf("run original: %v", err)
			}
			files, err := modernDecompileTree(t, origDir)
			if err != nil {
				t.Fatalf("decompile: %v", err)
			}
			rebuiltDir := filepath.Join(root, eval.name+"-rebuilt")
			if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
				t.Fatal(err)
			}
			var rebuiltPaths []string
			for name, source := range files {
				if strings.Contains(source, "yak-decompiler: undecompilable") || strings.Contains(source, "decompile dump failed") {
					t.Fatalf("%s was stubbed:\n%s", name, firstLines(source, 6))
				}
				path := filepath.Join(rebuiltDir, name)
				if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				rebuiltPaths = append(rebuiltPaths, path)
			}
			if err := compileTreeAt(t, javac, rebuiltDir, rebuiltPaths, 21); err != nil {
				t.Fatalf("rebuilt tree does not compile: %v", err)
			}
			rebuilt, err := runMatrix(t, java, driverRoot, rebuiltDir, eval.name)
			if err != nil {
				t.Fatalf("rebuilt tree does not verify/run: %v", err)
			}
			if rebuilt != original {
				t.Fatalf("behavior mismatch\noriginal:\n%s\nrebuilt:\n%s", original, rebuilt)
			}
		})
	}
}

func firstLines(source string, n int) string {
	lines := strings.Split(source, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func compileTreeAt(t *testing.T, javac, dir string, paths []string, release int) error {
	t.Helper()
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", strconv.Itoa(release), "-d", dir}, paths...)
	cmd := exec.Command(javac, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("javac --release %d: %v\n%s", release, err, out)
	}
	return nil
}

func modernDecompile(raw []byte, resolve func(string) ([]byte, bool)) (classparser.DecompileResult, error, any) {
	var result classparser.DecompileResult
	var err error
	var panicked any
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = r
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
		defer cancel()
		result, err = classparser.DecompileWithOptions(raw, classparser.DecompileOptions{
			Mode:                classparser.Precision,
			TargetSourceVersion: 21,
			EnvSnapshot:         map[string]string{},
			Resolve:             resolve,
			Context:             ctx,
			Limits: classparser.Limits{
				MaxInputBytes: 1 << 20, MaxReadBytes: 1 << 20, MaxParseItems: 1_000_000,
				MaxGraphScans: 1_000_000, MaxGraphEdges: 1_000_000, MaxSetElementWork: 10_000_000,
				MaxRequestWork: 100_000_000, MaxOutputBytes: 1 << 20, MaxASTDepth: 10_000,
				MaxDiagnostics: 64, MaxNodeCopies: 1_000_000,
			},
		})
	}()
	return result, err, panicked
}

func modernDecompileTree(t *testing.T, origDir string) (map[string]string, error) {
	t.Helper()
	var paths []string
	err := filepath.Walk(origDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && strings.HasSuffix(path, ".class") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	rawByClass := map[string][]byte{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		object, err := classparser.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		rawByClass[object.GetClassName()] = raw
	}
	resolve := func(name string) ([]byte, bool) {
		raw, ok := rawByClass[name]
		return raw, ok
	}
	files := map[string]string{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		object, err := classparser.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		internal := object.GetClassName()
		result, err, panicked := modernDecompile(raw, resolve)
		if panicked != nil {
			return nil, fmt.Errorf("decompile %s panicked: %v", internal, panicked)
		}
		if err != nil {
			return nil, fmt.Errorf("decompile %s: %w", internal, err)
		}
		if len(result.StubMethods) != 0 || result.Source == "" {
			return nil, fmt.Errorf("decompile %s incomplete: status=%s stubs=%v diagnostics=%s",
				internal, result.Status, result.StubMethods, diagnosticText(result))
		}
		if result.Status != "complete" {
			// T21 may decline to pretty-print a pattern switch while still
			// emitting the equivalent instanceof dispatcher. Accept only the
			// documented equivalence diagnostics; a stub or an unknown
			// unsupported family still fails the shape.
			allowed := true
			for _, diagnostic := range result.Diagnostics {
				switch diagnostic.Code {
				case "bootstrap_partial", "legacy_source_recovery_disabled":
				default:
					allowed = false
				}
			}
			if !allowed {
				return nil, fmt.Errorf("decompile %s unsupported: status=%s diagnostics=%s",
					internal, result.Status, diagnosticText(result))
			}
		}
		simple := internal
		if slash := strings.LastIndex(simple, "/"); slash >= 0 {
			simple = simple[slash+1:]
		}
		files[simple+".java"] = result.Source
	}
	return files, nil
}

const recordDeconstructionSource = `
public class RecDeconstruction {
    sealed interface Shape permits Circle, Rect {}
    record Circle(double r) implements Shape {}
    record Rect(double w, double h) implements Shape {}
    static double area(Shape s) {
        return switch (s) {
            case Circle(double r) -> r * r * 3;
            case Rect(double w, double h) -> w * h;
        };
    }
    public static void main(String[] a) {
        System.out.println(area(new Circle(2)));
        System.out.println(area(new Rect(2, 3)));
    }
}
`

// Java 21 record deconstruction puts record accessor calls under shared
// Throwable handlers that wrap failures in MatchException. Until that shared
// handler shape is structured, the method must degrade to an explicit stub
// with a diagnostic; a compiled source that silently drops an arm is worse.
func TestAdversarialRecordDeconstructionFailsClosed(t *testing.T) {
	if !envEnabled() {
		t.Skip("set JAVAJIVE_ADVERSARIAL_EVAL=1 to run the adversarial evaluation")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	if javacMajor(t, javac) < 21 {
		t.Skip("record deconstruction requires javac 21")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "RecDeconstruction.java")
	if err := os.WriteFile(path, []byte(recordDeconstructionSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := compileTreeAt(t, javac, dir, []string{path}, 21); err != nil {
		t.Fatalf("compile original: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "RecDeconstruction.class"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".class") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		object, err := classparser.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		resolver[object.GetClassName()] = data
	}
	result, err, panicked := modernDecompile(raw, func(name string) ([]byte, bool) {
		data, ok := resolver[name]
		return data, ok
	})
	if panicked != nil {
		t.Fatalf("decompiler panicked: %v", panicked)
	}
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	if result.Status == "complete" && len(result.StubMethods) == 0 {
		if strings.Contains(result.Source, "yak-decompiler: undecompilable") {
			t.Fatal("complete result still contains a stub marker")
		}
		if !strings.Contains(result.Source, "switch") {
			t.Fatalf("record-deconstruction shape lost its switch:\n%s", result.Source)
		}
		return
	}
	if !strings.Contains(result.Source, "yak-decompiler: undecompilable") {
		t.Fatalf("incomplete record-deconstruction shape did not fail closed:\n%s", result.Source)
	}
	if diagnostics := diagnosticText(result); !strings.Contains(diagnostics, "exception-handler structuring failed") {
		t.Fatalf("unexpected fail-closed diagnostic: %s", diagnostics)
	}
}
