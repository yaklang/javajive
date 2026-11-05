package adversarial

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cf "github.com/yaklang/javajive/classparser/classfilebuilder"
)

// buildDeepGotoChain emits a 2000-block linear goto chain. It stresses block
// count, label layout and source emission without any irreducible region.
func buildDeepGotoChain(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		const blocks = 2000
		labels := make([]*cf.Label, blocks)
		for i := range labels {
			labels[i] = code.NewLabel()
		}
		code.Load(cf.IntLocal, 0).Store(cf.IntLocal, 2)
		for i, label := range labels {
			code.Mark(label)
			code.IInc(2, 1)
			if i+1 < len(labels) {
				code.Branch(cf.GOTO, labels[i+1])
			}
		}
		code.Load(cf.IntLocal, 2).Emit(cf.IRETURN)
		return nil
	})
}

// buildDenseSwitch200 emits a 200-way tableswitch whose cases are distinct
// terminal blocks.
func buildDenseSwitch200(class *cf.Class) error {
	return addRun(class, func(code *cf.Code) error {
		const cases = 200
		labels := make([]*cf.Label, cases)
		for i := range labels {
			labels[i] = code.NewLabel()
		}
		fallback := code.NewLabel()
		code.Load(cf.IntLocal, 0).TableSwitch(0, fallback, labels...)
		for i, label := range labels {
			code.Mark(label)
			if err := pushInt(class, code, i+1); err != nil {
				return err
			}
			code.Emit(cf.IRETURN)
		}
		code.Mark(fallback)
		code.Emit(cf.ICONST_M1).Emit(cf.IRETURN)
		return nil
	})
}

func manySequentialIfsSource(count int) string {
	var body strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&body, "            if (x > %d) acc += %d;\n", i, (i%7)+1)
	}
	return fmt.Sprintf(`
public class ManySequentialIfs300 {
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
            int x = entry + i;
%s        }
        return acc;
    }
}`, body.String())
}

func manyCatchSource(count int) string {
	var body strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&body, "            try { acc += risky(entry + i + %d); } catch (IllegalStateException e) { acc += %d; }\n", i, (i%5)+1)
	}
	return fmt.Sprintf(`
public class DeepCatch50 {
    static int risky(int x) {
        if (x %% 7 == 0) throw new IllegalStateException("seven");
        return x;
    }
    public static int run(int entry, int budget) {
        int acc = 0;
        for (int i = 0; i < budget; i++) {
%s        }
        return acc;
    }
}`, body.String())
}

func wideLocalsSource(count int) string {
	var decls, uses strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&decls, "        int v%d = entry + %d;\n", i, i)
		fmt.Fprintf(&uses, "        acc += v%d;\n", i)
	}
	return fmt.Sprintf(`
public class WideLocals250 {
    public static int run(int entry, int budget) {
        int acc = budget;
%s%s        return acc;
    }
}`, decls.String(), uses.String())
}

type scaleRecord struct {
	name    string
	outcome string
	lines   int
	bytes   int
	elapsed time.Duration
}

func TestAdversarialScale(t *testing.T) {
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

	// Builder cases: class bytes are generated directly.
	builderCases := []struct {
		name  string
		build func(*cf.Class) error
	}{
		{"DeepGotoChain2000", buildDeepGotoChain},
		{"DenseSwitch200", buildDenseSwitch200},
	}
	// Source cases: javac emits the stress shape.
	sourceStress := []struct {
		name   string
		source string
	}{
		{"ManySequentialIfs300", manySequentialIfsSource(300)},
		{"DeepCatch50", manyCatchSource(50)},
		{"WideLocals250", wideLocalsSource(250)},
	}

	var records []scaleRecord
	run := func(name string, raw []byte, original string) {
		rec := scaleRecord{name: name}
		result, err, panicked, elapsed := decompile(raw)
		rec.elapsed = elapsed
		rec.lines = strings.Count(result.Source, "\n") + 1
		rec.bytes = len(result.Source)
		switch {
		case panicked != nil:
			rec.outcome = "panic"
			t.Errorf("decompiler panicked: %v", panicked)
		case err != nil:
			rec.outcome = "error"
			t.Errorf("decompile: %v", err)
		case result.Status != "complete" || len(result.StubMethods) != 0:
			rec.outcome = "incomplete"
			t.Errorf("status=%s stubs=%v diagnostics=%s", result.Status, result.StubMethods, diagnosticText(result))
		default:
			rebuiltDir := filepath.Join(root, name+"-rebuilt")
			if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := writeSource(t, rebuiltDir, name, result.Source)
			if err := compileJavaFile(t, javac, rebuiltDir, path); err != nil {
				rec.outcome = "compile-failed"
				t.Errorf("rebuilt source does not compile: %v", err)
			} else if rebuilt, runErr := runMatrix(t, java, driverRoot, rebuiltDir, name); runErr != nil {
				rec.outcome = "verify-failed"
				t.Errorf("rebuilt class does not verify/run: %v", runErr)
			} else if rebuilt != original {
				rec.outcome = "behavior-mismatch"
				t.Errorf("behavior mismatch")
			} else {
				rec.outcome = "recovered"
			}
		}
		records = append(records, rec)
		t.Logf("%-24s outcome=%-16s lines=%-7d bytes=%-8d elapsed=%s",
			rec.name, rec.outcome, rec.lines, rec.bytes, rec.elapsed.Round(time.Millisecond))
	}

	for _, eval := range builderCases {
		eval := eval
		t.Run(eval.name, func(t *testing.T) {
			raw := buildClass(t, eval.name, eval.build)
			dir := filepath.Join(root, eval.name+"-orig")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, eval.name+".class"), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := loadClass(t, java, driverRoot, dir, eval.name); err != nil {
				t.Fatalf("original class does not verify: %v", err)
			}
			original, err := runMatrix(t, java, driverRoot, dir, eval.name)
			if err != nil {
				t.Fatalf("original class does not run: %v", err)
			}
			run(eval.name, raw, original)
		})
	}

	for _, eval := range sourceStress {
		eval := eval
		t.Run(eval.name, func(t *testing.T) {
			dir := filepath.Join(root, eval.name+"-orig")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, eval.name+".java")
			if err := os.WriteFile(path, []byte(eval.source), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := compileTree(t, javac, dir, []string{path}); err != nil {
				t.Fatalf("compile original: %v", err)
			}
			original, err := runMatrix(t, java, driverRoot, dir, eval.name)
			if err != nil {
				t.Fatalf("run original: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, eval.name+".class"))
			if err != nil {
				t.Fatal(err)
			}
			run(eval.name, raw, original)
		})
	}

	t.Log("\n=== ADVERSARIAL SUMMARY (scale) ===")
	t.Log("| case | outcome | lines | bytes | ms |")
	t.Log("|---|---|---:|---:|---:|")
	for _, rec := range records {
		t.Logf("| %s | %s | %d | %d | %d |", rec.name, rec.outcome, rec.lines, rec.bytes, rec.elapsed.Milliseconds())
	}
}
