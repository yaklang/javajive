package cross

// 承重测试:「split-slot 拷贝临时变量丢失数组类型退化为 java.lang.Object」治本 (CODEC_TODO disjoint 槽族).
//
// 真实 fastjson2 ObjectReaderException.readObject (StackTraceElement[] foreach 去糖):
//   StackTraceElement[] stackTrace = ...;       // 槽 10, `= null` 起始 -> Object, 后 checkcast 采纳数组类型
//   for (StackTraceElement item : stackTrace)   // javac 去糖为:
//       StackTraceElement[] arr$ = stackTrace;  // 槽 19 (与更早一段 String[] 段 disjoint 复用)
//       int len$ = arr$.length; ...
//
// 拷贝 `arr$ = stackTrace` 的 aload 在 DFS 顺序里先于 stackTrace 的 checkcast-store 被访问,其 SlotValue
// 快照到源槽尚未采纳数组类型时的 java.lang.Object;NewVar 把这个陈旧 Object 冻结进拷贝目标(ref-68)。
// 于是 hoist 出的声明渲染成 `Object var18_1;`,`var18_1.length` -> "cannot find symbol: variable length",
// `var18_1[i]` -> "array required, but Object found"。治本 (PropagateCopyArrayDeclType 后处理:当一个局部
// 的唯一取值定义是对另一个局部的纯拷贝、且当前恰声明为 java.lang.Object、而源的实时类型是数组时,把目标
// 重打成该数组类型 —— 拷贝赋值恒等兼容、数组可赋给任意 Object 用点,无回归)。
// JDEC_COPY_ARRAY_DECL_TYPE_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// copyArrayDeclErrCount decompiles ObjectReaderException under the kill-switch, recompiles it against
// the jar, and returns the count of the two array-loss recompile errors: a `.length` read on a
// non-array variable ("cannot find symbol" + companion "symbol: variable length") and an index on a
// non-array variable ("array required, but Object found"). Both are the exact signature of a copy
// temp frozen to java.lang.Object; the file has unrelated `$`-nested-class import errors that the
// pairing/substring filters exclude, so the count isolates precisely this defect.
func copyArrayDeclErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_COPY_ARRAY_DECL_TYPE_OFF"
	const entry = "com/alibaba/fastjson2/reader/ObjectReaderException.class"
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
	src, err := jfs.ReadFile(entry) // JarFS.ReadFile decompiles on read (honoring the kill-switch).
	if err != nil {
		t.Fatalf("read %s: %v", entry, err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "ObjectReaderException.java")
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}

	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", jarPath, "-d", t.TempDir(), dst)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	lines := strings.Split(string(out), "\n")
	n := 0
	for i, line := range lines {
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "ObjectReaderException.java") {
			continue
		}
		// Index on a non-array: single-line "array required, but Object found".
		if strings.Contains(line, "array required") {
			n++
			continue
		}
		// `.length` on a non-array: "cannot find symbol" whose companion "symbol:" line names
		// a missing `variable length`.
		if strings.Contains(line, "cannot find symbol") {
			for j := i + 1; j < len(lines) && j <= i+4; j++ {
				if strings.Contains(lines[j], "symbol:") {
					if strings.Contains(lines[j], "variable length") {
						n++
					}
					break
				}
			}
		}
	}
	return n
}

// TestCopyArrayDeclTypeIsLoadBearing pins fastjson2 ObjectReaderException: a foreach copy temp
// (`arr$ = stackTrace`) whose single definition copies an array local must be declared with that
// array type, not the stale java.lang.Object frozen from a DFS-order SlotValue snapshot. Disabling
// the fix via the kill-switch must reintroduce the `.length`-on-Object and index-on-Object errors.
func TestCopyArrayDeclTypeIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}

	on := copyArrayDeclErrCount(t, jarPath, false) // fix ON
	off := copyArrayDeclErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("ObjectReaderException array-loss (Object copy temp) errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all array-loss errors: ON=%d (want 0)", on)
	}
}
