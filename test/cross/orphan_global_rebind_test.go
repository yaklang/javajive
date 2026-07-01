package cross

// 承重测试:「同一 JVM 槽的 store 在嵌套作用域被铸出新 id, 但其同 VarUid 的 READ 落在兄弟作用域(非该
// 铸造作用域的后代)时, 每个 rewriteVar 作用域只重写自己的语句切片, 兄弟作用域里的读保留 pre-mint 旧 id
// → 旧 id 无声明、渲染裸 varN 拼写, 与另一处同拼写的无关局部撞名」治本.
//
// 真实 fastjson2 残留: JSONPathParser.parseFilter
//   各 filter-segment 构造器的 name 实参在真实源里就是 (String)null(函数型 segment 的字段名为 null,
//   字节码 aconst_null;astore 7); 反编译把该 null 物化成 `Object var12 = null`(在 if(fieldName==null) 臂内
//   铸为 var12), 但其在运算符 switch(兄弟作用域)里的 16 处读保留旧 id, 渲染成裸 `var11`, 绑到后面的
//   `JSONPathFunction$SizeFunction var11` 声明 → `(String)(var11)` = (String)(SizeFunction)
//   → 16x "incompatible types: ... cannot be converted to String"。
//
// 治本(replayUnambiguousRebindings)把每个作用域 defer 的 (oldId->newId) 重绑累积到方法级共享表,
// 在 rewriteVar 递归结束后, 对「映射唯一」的 oldId 再做一次全方法 ReplaceVar, 补回兄弟作用域的孤儿读;
// 不相交槽(一个 oldId 重绑到多个 newId)自动跳过。kill-switch JDEC_ORPHAN_GLOBAL_REBIND_OFF=1 关掉必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classJavacErrorSubstr decompiles the named class entries, recompiles them against the jar, and
// returns the count of javac error lines containing substr. killOff toggles
// JDEC_ORPHAN_GLOBAL_REBIND_OFF around the decompile so the caller can compare fix-ON vs fix-OFF.
func classJavacErrorSubstr(t *testing.T, jarPath string, entries []string, fileSubstr, substr string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_ORPHAN_GLOBAL_REBIND_OFF"
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
		if strings.Contains(line, ": error:") && strings.Contains(line, substr) {
			if fileSubstr == "" || strings.Contains(line, fileSubstr) {
				n++
			}
		}
	}
	return n
}

// TestOrphanGlobalRebindIsLoadBearing pins the fastjson2 JSONPathParser.parseFilter orphan-read
// residual. With the fix ON the sibling-scope reads of the slot-7 null are rebound to their declared
// id (`var12`), so the cast renders `(String)(var12)` = (String)(Object) and compiles; disabling the
// cross-scope rebind replay via the kill-switch must reintroduce the "cannot be converted to String"
// errors.
//
// 必须把整个 JSONPath* 家族一起编译: 该 cast 的左操作数 `var11` 声明类型是内部类
// `JSONPathFunction$SizeFunction`, javajive 以独立顶层 `$` 名类文件发出; 若只单编 JSONPathParser.java,
// var11 的声明本身 "cannot find symbol" → cast 的 "cannot be converted to String" 被掩盖。整族同编后
// 兄弟 `$` 名类可解析, cast 错误才显形。
func TestOrphanGlobalRebindIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const prefix = "com/alibaba/fastjson2/JSONPath"
	var entries []string
	for _, e := range classEntries(t, jarPath) {
		if strings.HasPrefix(e, prefix) {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		t.Skip("no JSONPath* entries in fastjson2 jar; skipping")
	}
	const substr = "cannot be converted to String"
	const fileSubstr = "JSONPathParser.java"

	on := classJavacErrorSubstr(t, jarPath, entries, fileSubstr, substr, false) // fix ON
	off := classJavacErrorSubstr(t, jarPath, entries, fileSubstr, substr, true) // fix OFF (kill-switch)
	t.Logf("'cannot be converted to String' errors: ON=%d OFF=%d", on, off)

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
