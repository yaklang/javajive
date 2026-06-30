package cross

// 承重测试: 「布尔形参在分支里被 int 0/1 字面量重新赋值后于汇合处读取」治本 (CODEC_TODO disjoint 槽族).
//
// 真实 fastjson2 ObjectWriterProvider.getObjectWriterInternal: `boolean fieldBased` 在 `if` 内被
// `fieldBased = false;` 重新赋值, 随后 `fieldBased ? cacheFieldBased.get : cache.get` 读取。javac 把
// `fieldBased = false` 编成 `iconst_0; istore`(int 类别), 而该槽的到达定义是「方法入口形参」而非 store,
// reachingBoolSiblingArmMerge 的 store-only phi 判据无法锚定它, 于是 int-0/1 store 被拆成独立 int 变量,
// 汇合读以槽深派生的 varN 命名并与同深度的无关引用局部撞名 → "bad operand types for '!=': String, int"
// (本方法一处就 7 行)。治本: 让 store 续用布尔形参(0/1 转 false/true)。
// JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classBadOperandErrors decompiles the named class entries from jarPath, recompiles them against the
// jar, and returns the count of "bad operand types for binary operator" javac errors. killOff toggles
// JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF around the decompile so the caller can compare fix-ON vs fix-OFF.
func classBadOperandErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF")
	if killOff {
		os.Setenv("JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF", "1")
	} else {
		os.Unsetenv("JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF")
	}
	defer func() {
		if had {
			os.Setenv("JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF", prev)
		} else {
			os.Unsetenv("JDEC_BOOL_PARAM_REASSIGN_MERGE_OFF")
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
	return strings.Count(string(out), "error: bad operand types for binary operator")
}

// TestBoolParamReassignMergeIsLoadBearing pins fastjson2 ObjectWriterProvider: the boolean parameter
// `fieldBased`, reassigned `= false` inside an if and read in a later ternary, must continue the
// parameter (one boolean variable) rather than split off an int that collides with the String name
// local. Disabling the fix via the kill-switch must reintroduce "bad operand types" errors.
func TestBoolParamReassignMergeIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{"com/alibaba/fastjson2/writer/ObjectWriterProvider.class"}

	on := classBadOperandErrors(t, jarPath, entries, false) // fix ON
	off := classBadOperandErrors(t, jarPath, entries, true)  // fix OFF (kill-switch)
	t.Logf("ObjectWriterProvider bad-operand errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
}
