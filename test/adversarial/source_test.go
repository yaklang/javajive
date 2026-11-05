package adversarial

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

type sourceCase struct {
	name    string
	sources map[string]string
}

// sourceCases are ordinary javac outputs with control-flow shapes that real
// reverse-engineering targets contain: string/enum switches, resource cleanup,
// labeled breaks, monitors, lambdas, duplicated finally, and flattened state
// machines.
func sourceCases() []sourceCase {
	return []sourceCase{
		{"StringSwitchShape", map[string]string{"StringSwitchShape.java": `
public class StringSwitchShape {
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            int k = (entry + i) & 3;
            String key;
            if (k == 0) key = "alpha";
            else if (k == 1) key = "beta";
            else if (k == 2) key = "gamma";
            else key = "delta";
            switch (key) {
                case "alpha": acc += 1; break;
                case "beta": acc += 2; break;
                case "gamma": acc += 4; break;
                case "delta": acc += 8; break;
                default: acc += 16;
            }
        }
        return acc;
    }
}`}},
		{"EnumSwitchShape", map[string]string{"EnumSwitchShape.java": `
public class EnumSwitchShape {
    enum Mode { A, B, C, D }
    public static int run(int entry, int budget) {
        int acc = 0;
        Mode[] all = Mode.values();
        for (int i = 0; i < budget; i++) {
            Mode m = all[Math.floorMod(entry + i, all.length)];
            switch (m) {
                case A: acc += 1; break;
                case B: acc += 2; break;
                case C: acc += 4; break;
                default: acc += 8;
            }
        }
        return acc;
    }
}`}},
		{"TryWithResourcesShape", map[string]string{"TryWithResourcesShape.java": `
public class TryWithResourcesShape {
    static final class Res implements AutoCloseable {
        final int delta;
        final boolean fail;
        Res(int delta, boolean fail) { this.delta = delta; this.fail = fail; }
        public void close() throws Exception { if (fail) throw new Exception("close"); }
    }
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            try (Res r = new Res(i, (entry + i) % 5 == 0)) {
                acc += (r.delta & 3) + 1;
            } catch (Exception e) {
                acc += 100;
            }
        }
        return acc;
    }
}`}},
		{"NestedLabeledShape", map[string]string{"NestedLabeledShape.java": `
public class NestedLabeledShape {
    public static int run(int entry, int budget) {
        int acc = 0;
        outer:
        for (int i = 0; i < budget; i++) {
            for (int j = 0; j < budget; j++) {
                acc += (i ^ j) & 3;
                if (((entry + i + j) & 7) == 0) continue outer;
                if (((entry + i * 3 + j) & 11) == 0) break outer;
                if (((entry + j) & 5) == 0) break;
            }
            acc += 7;
        }
        return acc;
    }
}`}},
		{"SynchronizedShape", map[string]string{"SynchronizedShape.java": `
public class SynchronizedShape {
    private static final Object LOCK = new Object();
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            synchronized (LOCK) {
                acc += (entry + i) & 3;
            }
        }
        return acc;
    }
}`}},
		{"LambdaCaptureShape", map[string]string{"LambdaCaptureShape.java": `
public class LambdaCaptureShape {
    interface Op { int apply(int x); }
    public static int run(int entry, int budget) {
        int base = entry & 3;
        Op op = x -> x + base;
        int acc = 0;
        for (int i = 0; i < budget; i++) acc += op.apply(i & 7);
        return acc;
    }
}`}},
		{"FinallyDupShape", map[string]string{"FinallyDupShape.java": `
public class FinallyDupShape {
    static int effects;
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            try {
                if (((entry + i) & 3) == 0) return acc + 1;
                acc += 2;
            } catch (RuntimeException e) {
                acc += 3;
            } finally {
                effects++;
            }
        }
        return acc;
    }
}`}},
		{"FlattenedStateShape", map[string]string{"FlattenedStateShape.java": `
public class FlattenedStateShape {
    public static int run(int entry, int budget) {
        int state = 0;
        int acc = 0;
        int counter = budget;
        while (true) {
            switch (state) {
                case 0:
                    if (counter <= 0) { state = 4; continue; }
                    acc += 1; counter--; state = (entry & 1) == 0 ? 1 : 2; continue;
                case 1: acc += 2; state = 3; continue;
                case 2: acc += 4; state = 1; continue;
                case 3: acc += 8; state = 0; continue;
                default: return acc * 10 + 7;
            }
        }
    }
}`}},
		{"OpaquePredicateShape", map[string]string{"OpaquePredicateShape.java": `
public class OpaquePredicateShape {
    static volatile boolean flag = true;
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            if (flag) acc += (entry + i) & 3;
            else acc -= 1;
            if (flag || entry == 999) acc += 1;
        }
        return acc;
    }
}`}},
		{"InterfaceDefaultShape", map[string]string{
			"ShapeOps.java": `
interface ShapeOps {
    int BASE = 3;
    default int adjust(int x) { return x + BASE; }
    static int clamp(int x) { return x < 0 ? 0 : (x > 7 ? 7 : x); }
}`,
			"InterfaceDefaultShape.java": `
public class InterfaceDefaultShape implements ShapeOps {
    public static int run(int entry, int budget) {
        InterfaceDefaultShape s = new InterfaceDefaultShape();
        int acc = 0;
        for (int i = 0; i < budget; i++) acc += s.adjust(ShapeOps.clamp((entry + i) & 7));
        return acc;
    }
}`}},
	}
}

func compileTree(t *testing.T, javac, dir string, paths []string) error {
	t.Helper()
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", "8", "-d", dir}, paths...)
	cmd := exec.Command(javac, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("javac: %v\n%s", err, out)
	}
	return nil
}

// decompileTree decompiles every class in origDir and returns the emitted
// sources keyed by output file name, plus the worst per-class status.
func decompileTree(t *testing.T, origDir string) (map[string]string, []classparser.DecompileResult, error) {
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
		return nil, nil, err
	}
	sort.Strings(paths)
	files := map[string]string{}
	var results []classparser.DecompileResult
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		object, err := classparser.Parse(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", path, err)
		}
		internal := object.GetClassName()
		result, err, panicked, _ := decompile(raw)
		if panicked != nil {
			return nil, nil, fmt.Errorf("decompile %s panicked: %v", internal, panicked)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("decompile %s: %w", internal, err)
		}
		if result.Status != "complete" || len(result.StubMethods) != 0 {
			return nil, nil, fmt.Errorf("decompile %s incomplete: status=%s stubs=%v diagnostics=%s",
				internal, result.Status, result.StubMethods, diagnosticText(result))
		}
		simple := internal
		if slash := strings.LastIndex(simple, "/"); slash >= 0 {
			simple = simple[slash+1:]
		}
		files[simple+".java"] = result.Source
		results = append(results, result)
	}
	return files, results, nil
}

func TestAdversarialSourceShapes(t *testing.T) {
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
	root := t.TempDir()
	driverRoot := filepath.Join(root, "driver")
	if err := os.MkdirAll(driverRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	buildDriver(t, javac, driverRoot)

	type sourceRecord struct {
		name     string
		outcome  string
		classes  int
		lines    int
		bytes    int
		dispatch int
		elapsed  time.Duration
	}
	var records []sourceRecord
	for _, eval := range sourceCases() {
		eval := eval
		t.Run(eval.name, func(t *testing.T) {
			rec := sourceRecord{name: eval.name}
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
			sort.Strings(sourcePaths)
			if err := compileTree(t, javac, origDir, sourcePaths); err != nil {
				t.Fatalf("compile original: %v", err)
			}
			original, err := runMatrix(t, java, driverRoot, origDir, eval.name)
			if err != nil {
				t.Fatalf("run original: %v", err)
			}

			start := time.Now()
			files, results, err := decompileTree(t, origDir)
			rec.elapsed = time.Since(start)
			rec.classes = len(results)
			if err != nil {
				rec.outcome = "decompile-failed"
				t.Errorf("decompile: %v", err)
				records = append(records, rec)
				return
			}
			var all []string
			for name, source := range files {
				rec.lines += strings.Count(source, "\n") + 1
				rec.bytes += len(source)
				rec.dispatch += countDispatchTransfers(source)
				all = append(all, name)
			}
			sort.Strings(all)
			rebuiltDir := filepath.Join(root, eval.name+"-rebuilt")
			if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
				t.Fatal(err)
			}
			var rebuiltPaths []string
			for _, name := range all {
				path := filepath.Join(rebuiltDir, name)
				if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
					t.Fatal(err)
				}
				rebuiltPaths = append(rebuiltPaths, path)
			}
			if err := compileTree(t, javac, rebuiltDir, rebuiltPaths); err != nil {
				rec.outcome = "compile-failed"
				for _, name := range all {
					t.Logf("--- %s ---\n%s", name, files[name])
				}
				t.Errorf("rebuilt tree does not compile: %v", err)
				records = append(records, rec)
				return
			}
			rebuilt, err := runMatrix(t, java, driverRoot, rebuiltDir, eval.name)
			if err != nil {
				rec.outcome = "verify-failed"
				t.Errorf("rebuilt tree does not verify/run: %v", err)
				records = append(records, rec)
				return
			}
			if rebuilt != original {
				rec.outcome = "behavior-mismatch"
				t.Errorf("behavior mismatch\noriginal:\n%s\nrebuilt:\n%s", original, rebuilt)
				records = append(records, rec)
				return
			}
			rec.outcome = "recovered"
			records = append(records, rec)
			t.Logf("%-24s classes=%d lines=%d dispatch=%d elapsed=%s",
				rec.name, rec.classes, rec.lines, rec.dispatch, rec.elapsed.Round(time.Millisecond))
		})
	}

	t.Log("\n=== ADVERSARIAL SUMMARY (javac source shapes) ===")
	t.Log("| case | outcome | classes | lines | dispatch | ms |")
	t.Log("|---|---|---:|---:|---:|---:|")
	for _, rec := range records {
		t.Logf("| %s | %s | %d | %d | %d | %d |", rec.name, rec.outcome, rec.classes, rec.lines, rec.dispatch, rec.elapsed.Milliseconds())
	}
}
