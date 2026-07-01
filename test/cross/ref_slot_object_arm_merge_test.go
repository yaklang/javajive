package cross

// 承重测试:「同一 JVM 槽的 current 变量声明为 java.lang.Object(万能超类), 某不相交臂用更具体的引用值
// 重赋(astore S;goto MERGE), 所有臂汇入同一 post-merge 读」治本。
//
// 真实 fastjson2 残留: ObjectWriterAdapter.toJSONObject (slot 8, LVT 实证就是一个 `Object fieldValue`)
//   Object fieldValue = fieldWriter.getFieldValue(object);                       // current = Object
//   if (fieldClass == Date.class) fieldValue = DateUtils.format((Date)fieldValue, format);  // 重赋 String
//   ...
//   if (fieldValue instanceof Map) jsonObject.putAll((Map) fieldValue);          // post-merge 读
//   else if (Collection.class.isAssignableFrom(fieldClass)...) c = (Collection) fieldValue;
//
// DFS 序里 Object 初值铸出 Object 变量, String 重赋臂被拆出 String 变量, post-merge 的 instanceof/cast
// 读绑到 String 变量 → `(Map)(String)` / `(Collection)(String)` = 3x "String cannot be converted to
// Map/Collection"。subtype-arm 合并(CrossClassDirectLUB 永不返回 Object)与 sibling-arm 合并
// (CommonSuperType==Object 命中 "strict sibling only" 闸门而 bail)都覆盖不到 Object-current 这条边。
//
// 治本(reachingRefSlotObjectArmMerge, shape A): current 恰为 Object 且 store 为更具体引用, 经 phi
// (slotDefPhiReachesLoad) 证两 def 共达同一 load, 则续用 current(Object)。永不改任何变量声明类型, 故
// 不会把具体局部加宽成 Object。kill-switch JDEC_REF_SLOT_OBJECT_SUPERTYPE_ARM_MERGE_OFF=1 关掉必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classConvErrCount decompiles the named class entries under kill-switch `sw` (held to "1" when killOff),
// recompiles them against the jar, and returns the count of javac error lines that (1) name fileSubstr
// and (2) contain any of the conversion substrings.
func classConvErrCount(t *testing.T, sw, jarPath string, entries []string, fileSubstr string, substrs []string, killOff bool) int {
	t.Helper()
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
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ": error:") {
			continue
		}
		if fileSubstr != "" && !strings.Contains(line, fileSubstr) {
			continue
		}
		for _, s := range substrs {
			if strings.Contains(line, s) {
				n++
				break
			}
		}
	}
	return n
}

// TestRefSlotObjectArmMergeIsLoadBearing pins the fastjson2 ObjectWriterAdapter.toJSONObject
// Object-supertype-arm residual. With the fix ON the String reformat arms reuse the Object
// `fieldValue` variable, so `(Map) fieldValue` / `(Collection) fieldValue` render on an Object and
// compile; disabling the merge via the kill-switch must reintroduce the "String cannot be converted
// to Map/Collection" errors.
func TestRefSlotObjectArmMergeIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const sw = "JDEC_REF_SLOT_OBJECT_SUPERTYPE_ARM_MERGE_OFF"
	const entry = "com/alibaba/fastjson2/writer/ObjectWriterAdapter.class"
	const fileSubstr = "ObjectWriterAdapter.java"
	substrs := []string{
		"String cannot be converted to Map",
		"String cannot be converted to Collection",
	}
	entries := []string{entry}

	on := classConvErrCount(t, sw, jarPath, entries, fileSubstr, substrs, false)  // fix ON
	off := classConvErrCount(t, sw, jarPath, entries, fileSubstr, substrs, true)  // fix OFF (kill-switch)
	t.Logf("ObjectWriterAdapter String->Map/Collection errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all conversion errors: ON=%d (want 0)", on)
	}
}
