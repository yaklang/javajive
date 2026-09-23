package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// In Compatibility mode, javac-lowered enum constructor arguments may spill
// initialized arrays into <clinit> locals. Rebuilding those locals as enum
// arguments must preserve array contents and visible side-effect order.
// Precision deliberately excludes this legacy source-recovery pipeline.
func TestEnumArrayConstructorArgsRoundTrip(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "EnumArrayArgsMain", map[string]string{
		"EnumArrayArgsMain.java": `public class EnumArrayArgsMain {
  enum Tag {
    FIRST(new int[]{mark(1), mark(2)}, mark(3)),
    SECOND(new int[]{mark(4)}, mark(5));

    static int sequence;
    final int[] values;
    final int stamp;

    Tag(int[] values, int stamp) {
      this.values = values;
      this.stamp = stamp;
    }

    static int mark(int value) {
      sequence = sequence * 10 + value;
      return value;
    }

    int total() {
      int total = stamp;
      for (int value : values) total += value;
      return total;
    }
  }

  public static void main(String[] args) {
    System.out.println(Tag.FIRST.total() + ":" + Tag.SECOND.total() + ":" + Tag.sequence);
  }
}`,
	})
	if strings.TrimSpace(original) != "6:9:12345" {
		t.Fatalf("fixture oracle changed: %q, want 6:9:12345", original)
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
	usedEnumRecovery := false
	for name, raw := range classes {
		result, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility, Resolve: resolver})
		if err != nil {
			t.Fatalf("decompile %s: %v", name, err)
		}
		if result.Source == "" {
			t.Fatalf("decompile %s produced empty source (status=%s)", name, result.Status)
		}
		for _, rule := range result.RulesApplied {
			if rule.Rule == "fixEnumClinitIllegalNew" {
				usedEnumRecovery = true
			}
		}
		allSource.WriteString(result.Source)
		allSource.WriteByte('\n')
		javaFile := filepath.Join(reDir, name+".java")
		if err := os.WriteFile(javaFile, []byte(result.Source), 0o644); err != nil {
			t.Fatal(err)
		}
		javaFiles = append(javaFiles, javaFile)
	}
	if !usedEnumRecovery {
		t.Fatal("compatibility decompilation did not report enum constant recovery")
	}
	source := allSource.String()
	if !strings.Contains(source, "FIRST(new int[]") || !strings.Contains(source, "SECOND(new int[]") {
		t.Fatalf("enum constant array arguments were not reconstructed:\n%s", source)
	}
	if strings.Contains(source, "FIRST = new Tag(") || strings.Contains(source, "SECOND = new Tag(") {
		t.Fatalf("enum <clinit> still instantiates enum constants:\n%s", source)
	}
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", "17", "-d", outDir}, javaFiles...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = reDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recompile compatibility output: %v\n%s\n----- source -----\n%s", err, out, source)
	}
	if got := t04RunJava(t, java, outDir, "EnumArrayArgsMain"); got != original {
		t.Fatalf("enum round-trip stdout mismatch: got %q, want %q", got, original)
	}
}

func TestZxingEnumMultipleSpilledArrayArgumentsRoundTrip(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "com.google.zxing.ZxingEnumArraySpillMain", map[string]string{
		"ZxingEnumArraySpillMain.java": `package com.google.zxing;
public class ZxingEnumArraySpillMain {
  enum Code {
    FIRST(new int[]{mark(1), mark(2)}, new String[0]),
    SECOND(new int[]{mark(3), mark(4)}, new String[]{markText(5)}),
    THIRD(new int[]{mark(6)}, new String[0]);

    static int sequence;
    final int[] values;
    final String[] aliases;

    Code(int[] values, String... aliases) {
      this.values = values;
      this.aliases = aliases;
    }

    static int mark(int value) {
      sequence = sequence * 10 + value;
      return value;
    }

    static String markText(int value) {
      sequence = sequence * 10 + value;
      return "alias-" + value;
    }

    int total() {
      int total = 0;
      for (int value : values) total += value;
      return total;
    }
  }

  public static void main(String[] args) {
    System.out.println(Code.FIRST.total() + ":" + Code.SECOND.total() + ":" +
        Code.THIRD.total() + ":" + Code.SECOND.aliases[0] + ":" + Code.sequence);
  }
}`,
	})
	want := "3:7:6:alias-5:123456\n"
	if original != want {
		t.Fatalf("independent javac/java oracle changed: got %q want %q", original, want)
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
	if !strings.Contains(allSource.String(), "SECOND(new int[]{mark(3),mark(4)}") || !strings.Contains(allSource.String(), "markText(5)") {
		t.Fatalf("enum spill arguments were not restored at their source positions:\n%s", allSource.String())
	}
	if strings.Contains(allSource.String(), "SECOND(var") || strings.Contains(allSource.String(), "THIRD(var") {
		t.Fatalf("enum constants still reference <clinit> locals:\n%s", allSource.String())
	}
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", "17", "-d", outDir}, javaFiles...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = reDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recompile compatibility output: %v\n%s\n----- source -----\n%s", err, out, allSource.String())
	}
	if got := t04RunJava(t, java, outDir, "com.google.zxing.ZxingEnumArraySpillMain"); got != want {
		t.Fatalf("enum spill runtime mismatch: got %q want %q", got, want)
	}
}

func TestZxingEnumArraySpillFoldRejectsReorderedOrEffectfulShapes(t *testing.T) {
	base := `package com.google.zxing;
enum X {
	FIRST,
	SECOND;
	static {
		int[] var0 = new int[]{mark(1)};
		String[] var1 = new String[]{markText(2)};
		FIRST = new X("FIRST",0,var0,var1);
		int[] var2 = new int[]{mark(3)};
		String[] var3 = new String[]{markText(4)};
		SECOND = new X("SECOND",1,var2,var3);
		$VALUES = new X[]{FIRST,SECOND};
	}
}
`
	got := foldEnumStaticNewIntoConstants(base)
	if !strings.Contains(got, `FIRST(new int[]{mark(1)},new String[]{markText(2)})`) || !strings.Contains(got, `SECOND(new int[]{mark(3)},new String[]{markText(4)})`) {
		t.Fatalf("safe multi-argument spills were not folded in order:\n%s", got)
	}
	if strings.Contains(got, "var0") || strings.Contains(got, "var1") || strings.Contains(got, "var2") || strings.Contains(got, "var3") {
		t.Fatalf("consumed spill locals remain:\n%s", got)
	}

	unsafe := map[string]string{
		"reversed arguments":                    strings.Replace(base, `FIRST = new X("FIRST",0,var0,var1);`, `FIRST = new X("FIRST",0,var1,var0);`, 1),
		"visible call between spills":           strings.Replace(base, "\t\tString[] var1", "\t\tmark(9);\n\t\tString[] var1", 1),
		"repeated local":                        strings.Replace(base, `FIRST = new X("FIRST",0,var0,var1);`, `FIRST = new X("FIRST",0,var0,var0);`, 1),
		"side effect in unmapped argument":      strings.Replace(base, `FIRST = new X("FIRST",0,var0,var1);`, `FIRST = new X("FIRST",0,mark(9),var0,var1);`, 1),
		"unmapped allocation between constants": strings.Replace(base, "\t\tint[] var2", "\t\tint[] extra = new int[0];\n\t\tint[] var2", 1),
	}
	for name, input := range unsafe {
		t.Run(name, func(t *testing.T) {
			if output := foldEnumStaticNewIntoConstants(input); output != input {
				t.Fatalf("unsafe enum initializer was moved:\n%s", output)
			}
		})
	}
}

func TestEnumClinitArrayTempDoesNotMoveAcrossEffects(t *testing.T) {
	cases := map[string]string{
		"intervening side effect": "enum X {\n\tFIRST;\n\tstatic {\n\t\tint[] var0 = new int[]{mark(1)};\n\t\tmark(2);\n\t\tFIRST = new X(\"FIRST\",0,var0);\n\t\t$VALUES = $values();\n\t}\n}\n",
		"repeated consumer":       "enum X {\n\tFIRST;\n\tstatic {\n\t\tint[] var0 = new int[]{mark(1)};\n\t\tFIRST = new X(\"FIRST\",0,var0,var0);\n\t\t$VALUES = $values();\n\t}\n}\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			out := fixEnumClinitIllegalNew(in)
			if strings.Contains(out, "FIRST(new int[]") {
				t.Fatalf("moved an unsafe array initializer:\n%s", out)
			}
			if !strings.Contains(out, "int[] var0 = new int[]{mark(1)};") || !strings.Contains(out, "FIRST = new X(\"FIRST\",0,var0") {
				t.Fatalf("unsupported enum initializer shape was not preserved:\n%s", out)
			}
		})
	}
	t.Setenv("JDEC_ENUM_CLINIT_NEW_OFF", "1")
	if got := fixEnumClinitIllegalNew(cases["intervening side effect"]); got != cases["intervening side effect"] {
		t.Fatalf("kill-switch changed source:\n%s", got)
	}
}
