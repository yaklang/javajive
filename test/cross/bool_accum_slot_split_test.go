package cross

// 承重测试: 「布尔累加器复用不相交的 int 循环计数器槽位」治本 (reachingBoolAccumulatorSlotSplit).
//
// 真实 spring ASM ClassWriter.toByteArray: interfaces 循环的 int 计数器 `i`(slot 12)在循环结束后其槽位
// 被后续 `boolean hasFrames = false; hasFrames |= mw.hasFrames();` 复用。`hasFrames = false` 编成
// `iconst_0; istore 12`(int 类别), AssignVarGuarded 见其与仍停在 slot 12 表项里的(已死)int 计数器 int-
// 兼容遂续用同一 ref, 合并两个不相交活跃区; 该槽随后因 `flag |= Zcall` 累加定型为 boolean, 于是更早的
// 循环渲染成 `boolean < int` / `array[boolean]` / `boolean++`, javac 报「bad operand types / boolean
// cannot be converted to int / bad operand type boolean for '++'」(一处三行)。治本: 累加器初值 0 转
// 布尔 false, 让 AssignVarGuarded 铸新布尔 flag, int 计数器保留自身 ref。
// JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classBoolIntFlowErrors decompiles the named entries, recompiles against the jar, and counts the javac
// errors this fix targets (bad operand types / boolean<->int / bad '++' operand). killOff toggles
// JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF around the decompile for a fix-ON vs fix-OFF comparison.
func classBoolIntFlowErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF")
	if killOff {
		os.Setenv("JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF", "1")
	} else {
		os.Unsetenv("JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF")
	}
	defer func() {
		if had {
			os.Setenv("JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF", prev)
		} else {
			os.Unsetenv("JDEC_BOOL_ACCUM_SLOT_SPLIT_OFF")
		}
	}()

	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	dir := t.TempDir()
	var files []string
	for _, entry := range entries {
		src, err := jfs.ReadFile(entry) // JarFS.ReadFile decompiles on read (honoring the kill-switch).
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		base := strings.TrimSuffix(filepath.Base(entry), ".class")
		dst := filepath.Join(dir, base+".java")
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		files = append(files, dst)
	}

	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", jarPath, "-d", t.TempDir())
	args = append(args, files...)
	out, _ := exec.Command(javac, args...).CombinedOutput()
	s := string(out)
	return strings.Count(s, "error: bad operand types for binary operator") +
		strings.Count(s, "boolean cannot be converted to int") +
		strings.Count(s, "bad operand type boolean for unary operator")
}

// TestBoolAccumSlotSplitIsLoadBearing pins spring ASM ClassWriter: the interfaces loop's int counter,
// whose slot a later `boolean hasFrames = false; hasFrames |= ...` accumulator reuses, must stay int.
// Disabling the split via the kill-switch reintroduces the boolean/int flow errors on the loop.
func TestBoolAccumSlotSplitIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["spring"].relPath)
	if jarPath == "" {
		t.Skip("spring-core jar not found under ~/.m2; skipping")
	}
	entries := []string{"org/springframework/asm/ClassWriter.class"}

	on := classBoolIntFlowErrors(t, jarPath, entries, false) // fix ON
	off := classBoolIntFlowErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("ClassWriter bool/int flow errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
}
