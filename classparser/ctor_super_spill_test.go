package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuperDelegationTrailingArgumentSpills(t *testing.T) {
	originalSource := strings.Join([]string{
		"public class CtorSuperSpillMain extends Base {",
		"  static String label(long value) { System.out.print(\"L\"); return Long.toString(value); }",
		"  static int mark(int value) {",
		"    System.out.print(value == 5 ? \"A\" : \"B\");",
		"    return value;",
		"  }",
		"  CtorSuperSpillMain(int seed) {",
		"    super(String.format(\"%s\", label(seed)),",
		"        new Object[]{mark(seed), mark(seed + 1)});",
		"  }",
		"  CtorSuperSpillMain(long seed) {",
		"    super(String.format(\"%s\", label(seed)),",
		"        new Object[]{mark((int) seed), mark((int) seed + 1)});",
		"  }",
		"  public static void main(String[] args) { new CtorSuperSpillMain(5); new CtorSuperSpillMain(7L); }",
		"}",
		"class Base {",
		"  Base(String label, Object[] values) {",
		"    System.out.print(\":\" + label + \":\" + values[0] + \",\" + values[1] + \":\");",
		"  }",
		"}",
	}, "\n")
	want, _ := t04CompileRun(t, "17", "CtorSuperSpillMain", map[string]string{
		"CtorSuperSpillMain.java": originalSource,
	})
	if want != "LAB:5:5,6:LBB:7:7,8:" {
		t.Fatalf("independent javac/java oracle changed: got %q", want)
	}

	spilledSource := strings.Join([]string{
		"public class CtorSuperSpillMain extends Base {",
		"  static String label(long value) { System.out.print(\"L\"); return Long.toString(value); }",
		"  static int mark(int value) {",
		"    System.out.print(value == 5 ? \"A\" : \"B\");",
		"    return value;",
		"  }",
		"  CtorSuperSpillMain(int seed) {",
		"    super(String.format(\"%s\", var2), var3);",
		"    String var2 = label(seed);",
		"    Object[] var3 = new Object[]{mark(seed), mark(seed + 1)};",
		"  }",
		"  CtorSuperSpillMain(long seed) {",
		"    super(String.format(\"%s\", var5), var6);",
		"    String var5 = label(seed);",
		"    Object[] var6 = new Object[]{mark((int) seed), mark((int) seed + 1)};",
		"  }",
		"  public static void main(String[] args) { new CtorSuperSpillMain(5); new CtorSuperSpillMain(7L); }",
		"}",
		"class Base {",
		"  Base(String label, Object[] values) {",
		"    System.out.print(\":\" + label + \":\" + values[0] + \",\" + values[1] + \":\");",
		"  }",
		"}",
	}, "\n")
	fixed := fixCtorDelegationArgumentSpills(spilledSource)
	if !strings.Contains(fixed, "super(String.format(\"%s\", label(seed)),new Object[]{mark(seed), mark(seed + 1)});") ||
		!strings.Contains(fixed, "super(String.format(\"%s\", label(seed)),new Object[]{mark((int) seed), mark((int) seed + 1)});") ||
		strings.Contains(fixed, "String var2") || strings.Contains(fixed, "Object[] var3") ||
		strings.Contains(fixed, "String var5") || strings.Contains(fixed, "Object[] var6") {
		t.Fatalf("trailing super argument spills were not restored in place:\n%s", fixed)
	}
	javac, java := t04Tools(t)
	workDir, outDir := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(workDir, "CtorSuperSpillMain.java")
	if err := os.WriteFile(sourcePath, []byte(fixed), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "17", "-d", outDir, sourcePath)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recompile restored super constructors: %v\n%s\n----- source -----\n%s", err, out, fixed)
	}
	if got := t04RunJava(t, java, outDir, "CtorSuperSpillMain"); got != want {
		t.Fatalf("restored super constructors changed behavior: got %q want %q", got, want)
	}

	secondCtor := strings.Index(spilledSource, "  CtorSuperSpillMain(long seed)")
	mainMethod := secondCtor + strings.Index(spilledSource[secondCtor:], "  public static void main")
	singleCtorSource := spilledSource[:secondCtor] + spilledSource[mainMethod:]
	unsafe := map[string]string{
		"reversed arguments": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", var3), var2);", 1),
		"repeated spill": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", var2), var2);", 1),
		"intervening statement": strings.Replace(singleCtorSource,
			"    Object[] var3", "    mark(0);\n    Object[] var3", 1),
		"uninitialized local outside spill run": strings.Replace(singleCtorSource,
			"    Object[] var3 = new Object[]{mark(seed), mark(seed + 1)};",
			"    Object[] var3;\n    var3 = new Object[]{mark(seed), mark(seed + 1)};", 1),
		"side effect before spill": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", touch()), var3);", 1),
		"lambda capture would defer spill evaluation": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(() -> var2, var3);", 1),
		"unsigned shift assignment target instead of read": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", var2), (var3 >>>= 1));", 1),
		"qualified field instead of local": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", this.var2), var3);", 1),
		"method name instead of local": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", var2()), var3);", 1),
		"assignment target instead of read": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", var2 = \"changed\"), var3);", 1),
		"array element instead of array local": strings.Replace(singleCtorSource,
			"super(String.format(\"%s\", var2), var3);",
			"super(String.format(\"%s\", var2), var3[0]);", 1),
	}
	for name, input := range unsafe {
		t.Run(name, func(t *testing.T) {
			if output := fixCtorDelegationArgumentSpills(input); output != input {
				t.Fatalf("unsafe trailing spill shape was moved:\n%s", output)
			}
		})
	}
}
