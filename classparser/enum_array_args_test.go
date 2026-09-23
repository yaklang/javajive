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
