package cross

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jj "github.com/yaklang/javajive"
)

const sampleJava = `public class Sample {
    private int count;
    public String name;

    public Sample(int c) {
        this.count = c;
    }

    public int add(int a, int b) {
        return a + b;
    }

    public String greet(String who) {
        if (who == null) {
            return "hello";
        }
        return "hello " + who;
    }

    public int loopSum(int n) {
        int s = 0;
        for (int i = 0; i < n; i++) {
            s += i;
        }
        return s;
    }

    public int getCount() {
        return this.count;
    }
}
`

const helperJava = `public class Helper {
    public static int square(int x) {
        return x * x;
    }

    public boolean isEven(int x) {
        return x % 2 == 0;
    }
}
`

// TestCrossDecompileSingleClass compiles a non-trivial class with javac and
// verifies javajive can decompile it back into recognizable Java source.
func TestCrossDecompileSingleClass(t *testing.T) {
	dir := t.TempDir()
	compileJava(t, dir, map[string]string{"Sample.java": sampleJava})

	src, err := jj.Decompile(readClass(t, dir, "Sample.class"))
	if err != nil {
		t.Fatalf("Decompile: %v", err)
	}
	t.Logf("decompiled Sample:\n%s", src)

	for _, want := range []string{"class Sample", "add", "greet", "loopSum", "getCount"} {
		if !strings.Contains(src, want) {
			t.Errorf("decompiled source missing %q", want)
		}
	}
}

// TestCrossParseClass verifies the class parser agrees with javac output.
func TestCrossParseClass(t *testing.T) {
	dir := t.TempDir()
	compileJava(t, dir, map[string]string{"Sample.java": sampleJava})

	obj, err := jj.ParseClass(readClass(t, dir, "Sample.class"))
	if err != nil {
		t.Fatalf("ParseClass: %v", err)
	}
	if obj.GetClassName() != "Sample" {
		t.Fatalf("class name = %q", obj.GetClassName())
	}
	if obj.GetSupperClassName() != "java/lang/Object" {
		t.Fatalf("super = %q", obj.GetSupperClassName())
	}
	// constructor + 4 methods = 5
	if len(obj.Methods) < 5 {
		t.Fatalf("expected >=5 methods, got %d", len(obj.Methods))
	}
}

// TestCrossDecompileJar compiles multiple classes, packs them into a jar, and
// verifies the whole-archive decompile path produces a .java file per class.
func TestCrossDecompileJar(t *testing.T) {
	dir := t.TempDir()
	compileJava(t, dir, map[string]string{
		"Sample.java": sampleJava,
		"Helper.java": helperJava,
	})

	jarPath := filepath.Join(t.TempDir(), "sample.jar")
	zipClassesToJar(t, dir, jarPath)

	outDir := filepath.Join(t.TempDir(), "out")
	if err := jj.DecompileArchive(jarPath, outDir); err != nil {
		t.Fatalf("DecompileArchive: %v", err)
	}

	for _, name := range []string{"Sample.java", "Helper.java"} {
		found := false
		_ = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Base(path) == name {
				found = true
			}
			return nil
		})
		if !found {
			t.Errorf("expected decompiled %s under %s", name, outDir)
		}
	}
}
