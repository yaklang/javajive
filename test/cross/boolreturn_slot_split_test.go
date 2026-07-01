package cross

// 承重测试: 「布尔方法 return false/true 复用了不相交活跃区间的 int 槽」治本 (CODEC_TODO disjoint 槽族).
//
// 真实 guava LocalCache$Segment.replace(K,int,V,V): `int newCount = this.count - 1; ...;
// this.count = newCount;` 在 `if (valueRef.isActive())` 内使用 slot 14 作 int, 随后 `return false;`
// 被 javac 编成 `iconst_0; istore 14` 复用同一槽。因布尔 false/true 在 JVM 栈上是 int-0/1 字面量,
// AssignVarGuarded 视其与槽内 int ref 同类而续用, 把两个不相交活跃区间合并成一个变量; 之后 IRETURN
// 把该 ref 重定型为 boolean, 于是先前的 int 用法 (`newCount = count-1` / `this.count = newCount`)
// 变成 `boolean = int` / `int = boolean`, javac 拒绝 (本方法 3 行)。治本: 在该 store 把 0/1 转成
// boolean, 让 AssignVarGuarded 新铸一个 boolean 变量, 把 return 区间从 int 区间拆开。
// JDEC_BOOL_RETURN_SLOT_SPLIT_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classIntBoolErrors decompiles the named class entries from jarPath, recompiles them against the
// jar, and returns the count of int<->boolean "incompatible types" javac errors. killOff toggles
// JDEC_BOOL_RETURN_SLOT_SPLIT_OFF around the decompile so the caller can compare fix-ON vs fix-OFF.
func classIntBoolErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_BOOL_RETURN_SLOT_SPLIT_OFF")
	if killOff {
		os.Setenv("JDEC_BOOL_RETURN_SLOT_SPLIT_OFF", "1")
	} else {
		os.Unsetenv("JDEC_BOOL_RETURN_SLOT_SPLIT_OFF")
	}
	defer func() {
		if had {
			os.Setenv("JDEC_BOOL_RETURN_SLOT_SPLIT_OFF", prev)
		} else {
			os.Unsetenv("JDEC_BOOL_RETURN_SLOT_SPLIT_OFF")
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
	text := string(out)
	return strings.Count(text, "int cannot be converted to boolean") +
		strings.Count(text, "boolean cannot be converted to int")
}

// TestBoolReturnSlotSplitIsLoadBearing pins guava LocalCache$Segment: a boolean method's
// `return false/true` reusing a JVM slot that a disjoint earlier range used as an int local must
// split into two variables (int range + boolean return) rather than merge into one. Disabling the
// fix via the kill-switch must reintroduce the int<->boolean "incompatible types" errors.
func TestBoolReturnSlotSplitIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}
	entries := []string{"com/google/common/cache/LocalCache$Segment.class"}

	on := classIntBoolErrors(t, jarPath, entries, false) // fix ON
	off := classIntBoolErrors(t, jarPath, entries, true)  // fix OFF (kill-switch)
	t.Logf("LocalCache$Segment int<->boolean errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
}
