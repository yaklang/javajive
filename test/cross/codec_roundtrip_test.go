package cross

// 语义差分门禁, 对应 CODEC_TODO 里引用的 TestCodecSemanticsRoundTrip。流程:
//
//	javac 编译原始 .java  ->  反编译每个 .class 为新 .java  ->  javac 重编译  ->
//	分别 java 运行原始与重编译产物, 断言运行时输出 byte-identical (指纹一致)。
//
// 这是「能编译」之上更强的「语义正确」闸门: 控制流/数据流被破坏但仍能编译的反编译 bug
// (例如标号 continue 退化成普通 continue 导致死循环, 或 then/else 对调) 只有运行才暴露。
// 每个算法形态一个 battery (内联 .java 源), 跑前先编译原始确保种子本身有效。

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jj "github.com/yaklang/javajive"
)

// runTimeout bounds every `java` execution. A decompile bug can turn a finite loop into an infinite
// one (Bug AJ: labeled continue degraded to a plain continue); without a bound the whole suite would
// hang. On timeout the run is reported as a semantic failure, never a hang.
const runTimeout = 20 * time.Second

// decompileClassesInDir decompiles every .class under srcDir and writes each as <SimpleName>.java
// into dstDir (flat layout; batteries are package-less top-level classes plus their inner classes).
func decompileClassesInDir(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".class") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src, err := jj.Decompile(data)
		if err != nil {
			t.Fatalf("decompile %s: %v", filepath.Base(path), err)
		}
		base := strings.TrimSuffix(filepath.Base(path), ".class") // keep flat Outer$Inner name
		dst := filepath.Join(dstDir, base+".java")
		if err := os.WriteFile(dst, []byte(src), 0o644); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", srcDir, err)
	}
}

// compileDir compiles every .java directly under dir (flat) into the same dir, failing the test on
// javac error (it returns the combined output so the caller can surface the decompiled source).
func compileDir(t *testing.T, dir string) (string, bool) {
	t.Helper()
	javac := lookJavac(t)
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".java") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	args := append([]string{"-encoding", "UTF-8", "--release", "8", "-nowarn", "-d", dir}, files...)
	cmd := exec.Command(javac, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// runMain runs `java -cp dir mainClass args...` under runTimeout and returns (output, timedOut). A
// non-zero exit (that is not a timeout) fails the test; a timeout is returned so the caller can treat
// it as a semantic round-trip failure (an introduced infinite loop) rather than hanging the suite.
func runMain(t *testing.T, dir, mainClass string, args ...string) (string, bool) {
	t.Helper()
	java := lookJava(t)
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	full := append([]string{"-cp", dir, mainClass}, args...)
	cmd := exec.CommandContext(ctx, java, full...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), true
	}
	if err != nil {
		t.Fatalf("java %s in %s failed: %v\n%s", mainClass, dir, err, out)
	}
	return string(out), false
}

// mainFQN finds the fully-qualified name of simpleName by locating <simpleName>.class under dir and
// turning its package-relative path into a dotted name. The decompiler emits a `defaultpackagename`
// package for default-package classes, so the recompiled main class moves out of the root; deriving
// the FQN from the compiled layout keeps the runner agnostic to that.
func mainFQN(t *testing.T, dir, simpleName string) string {
	t.Helper()
	fqn := simpleName
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Base(path) != simpleName+".class" {
			return nil
		}
		rel, e := filepath.Rel(dir, path)
		if e == nil {
			fqn = strings.TrimSuffix(filepath.ToSlash(rel), ".class")
			fqn = strings.ReplaceAll(fqn, "/", ".")
		}
		return nil
	})
	return fqn
}

// assertSemanticRoundTrip is the core differential gate: compile the original sources, decompile,
// recompile the decompiled source, then run both and require byte-identical runtime output.
func assertSemanticRoundTrip(t *testing.T, mainClass string, args []string, sources map[string]string) {
	t.Helper()
	lookJavac(t)
	lookJava(t)

	origDir := t.TempDir()
	compileJava(t, origDir, sources) // fails the test if the seed itself is invalid

	decDir := t.TempDir()
	decompileClassesInDir(t, origDir, decDir)

	if out, ok := compileDir(t, decDir); !ok {
		// Surface the decompiled main source to make the failure diagnosable.
		dump, _ := os.ReadFile(filepath.Join(decDir, mainClass+".java"))
		t.Fatalf("recompiling decompiled source failed:\n%s\n--- decompiled %s.java ---\n%s",
			out, mainClass, dump)
	}

	origOut, origTO := runMain(t, origDir, mainFQN(t, origDir, mainClass), args...)
	if origTO {
		t.Fatalf("original %s timed out (seed itself loops; fix the battery)", mainClass)
	}
	decOut, decTO := runMain(t, decDir, mainFQN(t, decDir, mainClass), args...)
	if decTO {
		t.Errorf("semantic round-trip FAILURE for %s: recompiled code timed out "+
			"(decompile introduced an infinite loop)", mainClass)
		return
	}
	if origOut != decOut {
		t.Errorf("semantic round-trip MISMATCH for %s:\noriginal:\n%q\nrecompiled:\n%q",
			mainClass, origOut, decOut)
	}
}

// phaseTarget skips a battery that is a known forward target for a later phase, unless the matching
// env override is set (so the phase's implementer can run it as the differential signal). It keeps
// the Phase 0 baseline green while preserving the discovered bug as an executable target.
func phaseTarget(t *testing.T, env, bug string) {
	t.Helper()
	if os.Getenv(env) == "" {
		t.Skipf("known target (%s); set %s=1 to run it as the differential signal", bug, env)
	}
}

// --- representative batteries (seed of the differential gate; more added per phase) ---

// TestRoundTripControlFlow exercises loops, nested if/else, switch and a labeled continue/break.
// It currently FAILS: the decompiler drops the `for` increment on the fall-through path and renders
// `continue outer` as an empty block, turning the finite loop into an infinite one. This is the
// synthetic finite-for differential seed for Phase 4 (Bug AJ/AM); run with JDEC_RUN_PHASE4=1.
func TestRoundTripControlFlow(t *testing.T) {
	phaseTarget(t, "JDEC_RUN_PHASE4", "Bug AJ/AM: for-increment + labeled-continue reconstruction")
	const src = `public class CtrlFlow {
    static int classify(int n) {
        int acc = 0;
        outer:
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if (j == i) { continue; }
                if (i * j > 12) { continue outer; }
                if (i + j == 7) { acc += 100; break; }
                switch ((i + j) % 3) {
                    case 0: acc += 1; break;
                    case 1: acc += 2; break;
                    default: acc += 3;
                }
            }
            acc += i;
        }
        return acc;
    }

    public static void main(String[] a) {
        StringBuilder sb = new StringBuilder();
        for (int n = 0; n <= 8; n++) {
            sb.append(n).append('=').append(classify(n)).append(';');
        }
        System.out.println(sb.toString());
    }
}
`
	assertSemanticRoundTrip(t, "CtrlFlow", nil, map[string]string{"CtrlFlow.java": src})
}

// TestRoundTripTea exercises an integer block cipher (TEA): bit-shifts, additions, hex formatting —
// a pure arithmetic algorithm whose any miscompiled operator flips the fingerprint.
func TestRoundTripTea(t *testing.T) {
	const src = `public class Tea {
    static void encrypt(int[] v, int[] k) {
        int v0 = v[0], v1 = v[1], sum = 0;
        int delta = 0x9E3779B9;
        for (int i = 0; i < 32; i++) {
            sum += delta;
            v0 += ((v1 << 4) + k[0]) ^ (v1 + sum) ^ ((v1 >>> 5) + k[1]);
            v1 += ((v0 << 4) + k[2]) ^ (v0 + sum) ^ ((v0 >>> 5) + k[3]);
        }
        v[0] = v0; v[1] = v1;
    }

    public static void main(String[] a) {
        int[] k = {0x12345678, 0x9ABCDEF0, 0x0F1E2D3C, 0x4B5A6978};
        StringBuilder sb = new StringBuilder();
        for (int seed = 0; seed < 16; seed++) {
            int[] v = {seed * 2654435761L >>> 1 < 0 ? 1 : seed, ~seed};
            encrypt(v, k);
            sb.append(Integer.toHexString(v[0])).append(':').append(Integer.toHexString(v[1])).append(';');
        }
        System.out.println(sb.toString());
    }
}
`
	assertSemanticRoundTrip(t, "Tea", nil, map[string]string{"Tea.java": src})
}

// TestRoundTripTryFinally exercises try/catch/finally, string switch and exception flow — the areas
// where structural reconstruction can drop or reorder cleanup code.
func TestRoundTripTryFinally(t *testing.T) {
	const src = `public class TryFin {
    static String run(String cmd) {
        StringBuilder sb = new StringBuilder();
        try {
            switch (cmd) {
                case "add": sb.append(1 + 2); break;
                case "div": sb.append(10 / Integer.parseInt("0")); break;
                case "len": sb.append(cmd.length()); break;
                default: sb.append('?');
            }
        } catch (ArithmeticException e) {
            sb.append("AE");
        } catch (RuntimeException e) {
            sb.append("RE");
        } finally {
            sb.append("|fin");
        }
        return sb.toString();
    }

    public static void main(String[] a) {
        StringBuilder sb = new StringBuilder();
        for (String c : new String[]{"add", "div", "len", "x"}) {
            sb.append(run(c)).append('\n');
        }
        System.out.print(sb.toString());
    }
}
`
	assertSemanticRoundTrip(t, "TryFin", nil, map[string]string{"TryFin.java": src})
}
