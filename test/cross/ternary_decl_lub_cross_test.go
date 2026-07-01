package cross

// 承重测试: 「跨类(jar 内)三元 LUB 加宽」治本.
//
// 真实 fastjson2 残留(schema 包): 形如
//   `Any var6 = null; ...; var6 = cond ? Any.INSTANCE : Any.NOT_ANY;`
// 的局部, 其两臂都是 jar 内类 Any 的静态字段(Any extends JSONSchema). 旧 ternaryDeclLUB 只查 JDK
// 静态层级表, 不识别 jar 内 Any<:JSONSchema, 且仅在 isFirst(声明即初始化)时触发, 够不到这里的
// 「先 null 声明、后单独赋值」形态, 于是 var6 停在 Any, 而 javac 按 lub(Any.INSTANCE, Any.NOT_ANY) 仍是
// Any 之外的上界推断, 报 "incompatible types: bad type in conditional expression"
// (AllOf:56 / ArraySchema:133 / ObjectSchema:136,170 等)。
//
// 治本: ternaryDeclLUBCrossClass 借 ClassContext.SiblingSuperTypes 解析 jar 内直接父类型, 当两臂存在
// 直接(传递)子类型关系时把声明 ref 加宽到上界臂(仅 ResetVarType, 不触碰共享 ternary 的 cached type,
// 以免回流改窄某个以参数/this 为臂的 `?:` 实参)。
// kill-switch JDEC_TERNARY_DECL_LUB_CROSS_OFF=1 关掉加宽必复现这些 "bad type in conditional" 错误。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classBadConditionalErrors decompiles the named class entries, recompiles them against the jar, and
// returns the count of "bad type in conditional expression" errors. killOff toggles
// JDEC_TERNARY_DECL_LUB_CROSS_OFF around the decompile so the caller can compare fix-ON vs fix-OFF.
func classBadConditionalErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_TERNARY_DECL_LUB_CROSS_OFF"
	prev, had := os.LookupEnv(sw)
	if killOff {
		os.Setenv(sw, "1")
	} else {
		os.Unsetenv(sw)
	}
	defer func() {
		if had {
			os.Setenv(sw, prev)
		} else {
			os.Unsetenv(sw)
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
	return strings.Count(string(out), "bad type in conditional expression")
}

// TestTernaryDeclLUBCrossIsLoadBearing pins the fastjson2 schema cross-class ternary LUB residual.
// With the fix ON every `cond ? Any.INSTANCE : Any.NOT_ANY` whose declared local should widen to the
// jar-internal supertype (JSONSchema) must compile; disabling the cross-class widening via the
// kill-switch must reintroduce the "bad type in conditional expression" errors.
func TestTernaryDeclLUBCrossIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{
		"com/alibaba/fastjson2/schema/AllOf.class",
		"com/alibaba/fastjson2/schema/ArraySchema.class",
		"com/alibaba/fastjson2/schema/ObjectSchema.class",
	}

	on := classBadConditionalErrors(t, jarPath, entries, false) // fix ON
	off := classBadConditionalErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("'bad type in conditional expression' errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all cross-class ternary LUB errors: ON=%d (want 0)", on)
	}
}
