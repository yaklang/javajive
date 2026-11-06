package adversarial

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

func compileConstructorFixture(t *testing.T, javac, name, source string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name+".java")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := compileTreeAt(t, javac, dir, []string{path}, 21); err != nil {
		t.Fatalf("compile original: %v", err)
	}
	return dir
}

func constructorResolver(t *testing.T, dir string) ([]byte, func(string) ([]byte, bool)) {
	t.Helper()
	resolver := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var mainRaw []byte
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		object, err := classparser.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		resolver[object.GetClassName()] = raw
		if object.GetClassName() == "CtorPrefixSwitch" || strings.HasSuffix(object.GetClassName(), "/CtorPrefixSwitch") {
			mainRaw = raw
		}
	}
	if mainRaw == nil {
		t.Fatal("fixture class not found")
	}
	return mainRaw, func(name string) ([]byte, bool) {
		raw, ok := resolver[name]
		return raw, ok
	}
}

// A switch expression used as a super()/this() argument leaves a value on the
// operand stack before the delegation. Java cannot declare the synthetic join
// local there, so the method must fail closed instead of emitting cases that
// silently drop their value.
func TestAdversarialConstructorPrefixSwitchFailsClosed(t *testing.T) {
	if !envEnabled() {
		t.Skip("set JAVAJIVE_ADVERSARIAL_EVAL=1 to run the adversarial evaluation")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	if javacMajor(t, javac) < 21 {
		t.Skip("constructor switch expressions require javac 21")
	}
	const source = `
public class CtorPrefixSwitch extends CtorBase {
    CtorPrefixSwitch(int x) {
        super(switch (x) {
            case 0 -> "zero";
            case 1 -> "one";
            default -> "many";
        });
    }
    CtorPrefixSwitch(String s, int x) {
        this(switch (x) {
            case 1 -> 10;
            default -> 20;
        });
    }
    public static void main(String[] a) {
        System.out.println(new CtorPrefixSwitch(0).label);
    }
}
class CtorBase {
    final String label;
    CtorBase(String s) { this.label = s; }
}
`
	dir := compileConstructorFixture(t, javac, "CtorPrefixSwitch", source)
	raw, resolve := constructorResolver(t, dir)
	result, err, panicked := modernDecompile(raw, resolve)
	if panicked != nil {
		t.Fatalf("decompiler panicked: %v", panicked)
	}
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	if len(result.StubMethods) == 0 {
		t.Fatalf("prefix switch did not fail closed:\n%s", result.Source)
	}
	if !strings.Contains(result.Source, "yak-decompiler: undecompilable") {
		t.Fatalf("prefix switch returned apparent source without a stub:\n%s", result.Source)
	}
	if diagnostics := diagnosticText(result); !strings.Contains(diagnostics, "constructor prefix stack join cannot be expressed") {
		t.Fatalf("unexpected fail-closed diagnostic: %s", diagnostics)
	}
}

// A switch expression after the delegation is an ordinary local join and must
// round-trip, not be rejected with the prefix rule.
func TestAdversarialConstructorBodySwitchRoundTrip(t *testing.T) {
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
		t.Skip("constructor switch expressions require javac 21")
	}
	const source = `
public class CtorBodySwitch extends CtorBodyBase {
    String field;
    CtorBodySwitch(int x) {
        super("base");
        this.field = switch (x) {
            case 0 -> "zero";
            case 1 -> "one";
            default -> "many";
        };
    }
    public static void main(String[] a) {
        System.out.println(new CtorBodySwitch(0).field);
        System.out.println(new CtorBodySwitch(2).field);
    }
}
class CtorBodyBase {
    final String label;
    CtorBodyBase(String s) { this.label = s; }
}
`
	dir := compileConstructorFixture(t, javac, "CtorBodySwitch", source)
	files, err := modernDecompileTree(t, dir)
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	rebuilt := t.TempDir()
	var paths []string
	for name, decompiled := range files {
		if strings.Contains(decompiled, "yak-decompiler: undecompilable") {
			t.Fatalf("%s was stubbed:\n%s", name, decompiled)
		}
		path := filepath.Join(rebuilt, name)
		if err := os.WriteFile(path, []byte(decompiled), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	if err := compileTreeAt(t, javac, rebuilt, paths, 21); err != nil {
		t.Fatalf("rebuilt tree does not compile: %v", err)
	}
	out, err := exec.Command(java, "-Xverify:all", "-cp", rebuilt, "CtorBodySwitch").CombinedOutput()
	if err != nil {
		t.Fatalf("rebuilt constructor failed: %v\n%s", err, out)
	}
	if string(out) != "zero\nmany\n" {
		t.Fatalf("constructor behavior changed: %q", out)
	}
}
