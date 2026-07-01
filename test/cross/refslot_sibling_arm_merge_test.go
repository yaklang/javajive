package cross

// 承重测试: 「兄弟臂引用 phi 合并」治本.
//
// 真实 fastjson2 残留: FieldWriter.compareTo 源码
//   Member m1; if (cond) m1 = this.method; else m1 = this.field;  // Method / Field 两兄弟臂
//   ... if (m1 instanceof Method) ((Method) m1).getReturnType() ...
// 同一 JVM 槽在 if/else 两不相交臂分别存 Method 与 Field(都 <: Member, 无子类型关系), 汇合处多态使用。
// 旧 AssignVarGuarded 见两臂类型串不同(且非 null-init/形参/int 类), 把槽按类型拆成 Method 名与 Field 名,
// 汇合 use 绑到其中一臂(Field), 于是 `var6 instanceof Method`/`(Method) var6` 全部因 Field 永不可能是
// Method 而 javac 报 "inconvertible types: Field cannot be converted to Method"(本方法共 16 行)。
//
// 治本: reachingRefSlotSiblingArmMerge —— 当槽 current 与存储值都是非空非形参引用、类型不同、存在 JDK 表
// 非 Object 公共上界 L(且 L 严格是双方上界而非任一臂自身)、且 phi(下游 load 同时被本 store 与 current 的
// def 到达)证明同一变量时, 把 current 加宽到 L(此处 Method/Field → Member)并续用。CommonSuperType 对只共
// Object 的 Map/List/Number/String 派发型复用返回 nil, 故天然排除真不相交复用。
// kill-switch JDEC_REF_SLOT_SIBLING_ARM_MERGE_OFF=1 关掉合并必复现这些 Method/Field 互转错。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classReflectConvertErrors decompiles the named class entries, recompiles them against the jar, and
// returns the count of "cannot be converted to Method/Field" errors (the Method/Field sibling-merge
// signature). killOff toggles JDEC_REF_SLOT_SIBLING_ARM_MERGE_OFF around the decompile so the caller
// can compare fix-ON vs fix-OFF.
func classReflectConvertErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_REF_SLOT_SIBLING_ARM_MERGE_OFF"
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
	s := string(out)
	return strings.Count(s, "cannot be converted to Method") + strings.Count(s, "cannot be converted to Field")
}

// TestRefSlotSiblingArmMergeIsLoadBearing pins the fastjson2 FieldWriter.compareTo sibling-arm reflection
// phi. With the fix ON every Method/Field arm merges to Member so the polymorphic instanceof/cast uses
// compile; disabling the sibling-arm merge via the kill-switch must reintroduce the Method/Field
// inconvertible-type errors.
func TestRefSlotSiblingArmMergeIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{
		"com/alibaba/fastjson2/writer/FieldWriter.class",
	}

	on := classReflectConvertErrors(t, jarPath, entries, false) // fix ON
	off := classReflectConvertErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("'cannot be converted to Method/Field' errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all reflection sibling-merge errors: ON=%d (want 0)", on)
	}
}
