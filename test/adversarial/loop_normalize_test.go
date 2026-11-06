package adversarial

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The loop-form pass is a source-shape normalization, not a semantic rewrite:
// simple top-tested loops become for-loops, while loops with continue/break
// keep their explicit transfer shape, and the rebuilt program must match.
func TestAdversarialLoopFormRoundTrip(t *testing.T) {
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
	const name = "LoopForms"
	const source = `
public class LoopForms {
    static int simple(int n) {
        int s = 0;
        for (int i = 0; i < n; i++) {
            s += i;
        }
        return s;
    }
    static int withContinue(int n) {
        int s = 0;
        for (int i = 0; i < n; i++) {
            if ((i & 1) == 0) continue;
            s += i;
        }
        return s;
    }
    static int withBreak(int n) {
        int s = 0;
        for (int i = 0; i < n; i++) {
            if (i == 3) break;
            s += i;
        }
        return s;
    }
    static int nested(int n) {
        int s = 0;
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if (j == 2) continue;
                s += i * j;
            }
        }
        return s;
    }
    public static void main(String[] args) {
        System.out.println(simple(5));
        System.out.println(withContinue(7));
        System.out.println(withBreak(9));
        System.out.println(nested(4));
    }
}
`
	dir := compileConstructorFixture(t, javac, name, source)
	originalRun, err := exec.Command(java, "-Xverify:all", "-cp", dir, name).CombinedOutput()
	if err != nil {
		t.Fatalf("run original: %v\n%s", err, originalRun)
	}
	files, err := modernDecompileTree(t, dir)
	if err != nil {
		t.Fatalf("loop decompile: %v", err)
	}
	sourceText := files[name+".java"]
	if !strings.Contains(sourceText, "for(") {
		t.Fatalf("simple loop did not acquire a for form:\n%s", sourceText)
	}
	rebuilt := t.TempDir()
	var paths []string
	for fileName, decompiled := range files {
		if strings.Contains(decompiled, "yak-decompiler: undecompilable") {
			t.Fatalf("%s was stubbed:\n%s", fileName, decompiled)
		}
		path := filepath.Join(rebuilt, fileName)
		if err := os.WriteFile(path, []byte(decompiled), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	if err := compileTreeAt(t, javac, rebuilt, paths, 8); err != nil {
		t.Fatalf("loop tree does not compile: %v", err)
	}
	rebuiltRun, err := exec.Command(java, "-Xverify:all", "-cp", rebuilt, name).CombinedOutput()
	if err != nil {
		t.Fatalf("loop run failed: %v\n%s", err, rebuiltRun)
	}
	if string(rebuiltRun) != string(originalRun) {
		t.Fatalf("loop behavior changed: got %q want %q", rebuiltRun, originalRun)
	}
}
