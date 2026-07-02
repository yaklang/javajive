package cross

// 承重测试:「arraylength(`.length`)的 CustomValue 未转发 ReplaceVar」治本 (CODEC_TODO disjoint 槽族).
//
// 真实 fastjson2 ObjectReaderProvider.getObjectReaderInternal:
//   Type   rawType             = pt.getRawType();             // 槽 5 (Type)
//   Type[] actualTypeArguments = pt.getActualTypeArguments(); // 槽 6 (Type[])
//   ...
//   if (actualTypeArguments.length != 0 && ...) { ... }       // arraylength 读 槽 6
//   if (actualTypeArguments.length == 1 && ...) { ... }
//
// 槽 5(Type)与槽 6(Type[])在栈模拟阶段一度共享同一初始变量 id;rewriteVar 为槽 6 的 store 铸了新 id
// (var7),但 `.length` 读位于合并分支子作用域,其读取的 JavaRef 被捕获在 arraylength 生成的
// CustomValue 闭包里。该 CustomValue 由 NewCustomValue(string, type) 创建、未传 replaceFunc,于是
// CustomValue.ReplaceVar 是空操作:in-scope rewriteVar 与整方法 orphan-rebind 的 ReplaceVar 都无法进入
// 闭包更新操作数,读取遂保留旧 id、渲染成 `var6.length`(var6 是 Type)-> "cannot find symbol: variable
// length" x2。治本 (OP_ARRAYLENGTH 补 replaceFunc,转发 ReplaceVar 到操作数,与 OP_CHECKCAST /
// OP_INSTANCEOF / 数值转换 CustomValue 一致)。JDEC_ARRAYLENGTH_REPLACE_FWD_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// varLengthSymbolErrCount decompiles ObjectReaderProvider under the kill-switch, recompiles it against
// the jar, and returns the count of "cannot find symbol" errors whose companion "symbol:" line names a
// missing `variable length` -- i.e. a `.length` rendered on a NON-array variable. Standalone recompile
// produces unrelated cannot-find-symbol noise, so the two-line pairing isolates exactly this defect.
func varLengthSymbolErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_ARRAYLENGTH_REPLACE_FWD_OFF"
	const entry = "com/alibaba/fastjson2/reader/ObjectReaderProvider.class"
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
	dst := filepath.Join(dir, "ObjectReaderProvider.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "cannot find symbol") {
			continue
		}
		if !strings.Contains(line, "ObjectReaderProvider.java") {
			continue
		}
		// javac prints the missing name on a following "symbol:" line; scan the small window.
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			if strings.Contains(lines[j], "symbol:") {
				if strings.Contains(lines[j], "variable length") {
					n++
				}
				break
			}
		}
	}
	return n
}

// TestArrayLengthReplaceFwdIsLoadBearing pins fastjson2 ObjectReaderProvider: an array local's
// `.length` read, captured inside the arraylength CustomValue, must follow variable rebinds
// (rewriteVar / orphan rebind) so it renders on the array variable, not a stale same-slot scalar.
// Disabling the fix via the kill-switch must reintroduce the "cannot find symbol: variable length"
// errors (the `.length` rendered on a non-array `Type` variable).
func TestArrayLengthReplaceFwdIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}

	on := varLengthSymbolErrCount(t, jarPath, false) // fix ON
	off := varLengthSymbolErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("ObjectReaderProvider `.length` on non-array symbol errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all `.length` symbol errors: ON=%d (want 0)", on)
	}
}
