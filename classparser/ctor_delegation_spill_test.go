package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCtorDelegationArgumentSpillsRoundTrip(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "CtorDelegationSpillMain", map[string]string{
		"CtorDelegationSpillMain.java": `public class CtorDelegationSpillMain {
  static final class Pair<F, S> {
    final F first;
    final S second;
    Pair(F first, S second) { this.first = first; this.second = second; }
    F first() { System.out.print("A"); return first; }
    S second() { System.out.print("B"); return second; }
  }

  static final class Target {
    final double[] first;
    final double[] second;
    Target(Pair<double[], double[]> pair) { this(pair.first(), pair.second()); }
    Target(double[] first, double[] second) { this.first = first; this.second = second; }
    double total() { return first[0] + second[0]; }
  }

  public static void main(String[] args) {
    Target target = new Target(new Pair<>(new double[]{2}, new double[]{5}));
    System.out.println(":" + target.total());
  }
}`,
	})
	if original != "AB:7.0\n" {
		t.Fatalf("independent javac/java oracle changed: got %q", original)
	}
	javac, java := t04Tools(t)
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName
		if i := strings.LastIndexByte(internalName, '/'); i >= 0 {
			base = internalName[i+1:]
		}
		b, ok := classes[base]
		return b, ok
	}
	reDir, outDir := t.TempDir(), t.TempDir()
	var javaFiles []string
	var allSource strings.Builder
	for name, raw := range classes {
		result, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility, Resolve: resolver})
		if err != nil {
			t.Fatalf("decompile %s: %v", name, err)
		}
		allSource.WriteString(result.Source)
		allSource.WriteByte('\n')
		path := filepath.Join(reDir, name+".java")
		if err := os.WriteFile(path, []byte(result.Source), 0o644); err != nil {
			t.Fatal(err)
		}
		javaFiles = append(javaFiles, path)
	}
	source := allSource.String()
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", "17", "-d", outDir}, javaFiles...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = reDir
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recompile compatibility output: %v\n%s\n----- source -----\n%s", err, out, source)
	}
	if strings.Contains(source, "this(var2,") || !strings.Contains(source, "this(((double[])(var1.first())),((double[])(var1.second())));") {
		t.Fatalf("delegating constructor spills were not folded in evaluation order:\n%s", source)
	}
	if got := t04RunJava(t, java, outDir, "CtorDelegationSpillMain"); got != original {
		t.Fatalf("delegating constructor runtime mismatch: got %q want %q", got, original)
	}
}

func TestCtorDelegationArgumentSpillsRejectUnsafeShapes(t *testing.T) {
	base := `class X {
	X(Pair var1) {
		int var2 = first();
		int var3 = second();
		this(var2,var3);
	}
}
`
	if got := fixCtorDelegationArgumentSpills(base); !strings.Contains(got, "this(first(),second());") || strings.Contains(got, "int var2") || strings.Contains(got, "int var3") {
		t.Fatalf("ordered one-use spills were not folded:\n%s", got)
	}

	unsafe := map[string]string{
		"side effect before a later spill": strings.Replace(base, "this(var2,var3);", "this(mark(),var3);", 1),
		"field read before spill effects":  strings.Replace(base, "this(var2,var3);", "this(state,var2,var3);", 1),
		"statement between spills":         strings.Replace(base, "\t\tint var3", "\t\tmark();\n\t\tint var3", 1),
		"reversed arguments":               strings.Replace(base, "this(var2,var3);", "this(var3,var2);", 1),
		"repeated local":                   strings.Replace(base, "this(var2,var3);", "this(var2,var2);", 1),
		"local used after delegation":      strings.Replace(base, "\t}\n}", "\t\tSystem.out.println(var2);\n\t}\n}", 1),
		"commented constructor call":       strings.Replace(base, "this(var2,var3);", "this(var3,var2);\n\t\t/*\n\t\tthis(var2,var3);\n\t\t*/", 1),
	}
	for name, input := range unsafe {
		t.Run(name, func(t *testing.T) {
			if got := fixCtorDelegationArgumentSpills(input); got != input {
				t.Fatalf("unsafe spill shape was moved:\n%s", got)
			}
		})
	}
	commented := "class X {\n/*\nthis(var2,var3);\n*/\n}\n"
	commentedCall := strings.Index(commented, "this(")
	if javaCodePosition(commented, commentedCall) {
		t.Fatal("constructor-looking text inside a block comment was treated as Java code")
	}
	t.Setenv("JDEC_CTOR_DELEGATION_SPILLS_OFF", "1")
	if got := fixCtorDelegationArgumentSpills(base); got != base {
		t.Fatalf("kill switch changed source:\n%s", got)
	}
}
