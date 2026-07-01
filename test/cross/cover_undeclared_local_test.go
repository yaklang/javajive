package cross

// 承重测试: 「rewriteVar 把同一 JVM 槽拆成两个渲染成相同 varN 名的 *VariableId, 其中一个只被赋值/读取却
// 没有自己的声明」治本.
//
// 真实 fastjson2 三处残留:
//   - ObjectWriterAdapter.writeWithFilter 的 fieldWriter 循环: var19(FieldWriter)/var21(String) 在外层
//     直接使用, 而唯一的 `T varN` 声明却被沉到嵌套 else 臂里, 外层使用 "cannot find symbol: variable varN"。
//   - JSONReaderUTF16/UTF8.readOffsetDateTime: `char var7` 在 if 条件里内嵌赋值 (`(var7 = a[i]) != ' '`)
//     使用, 声明却在 else 臂里, 条件求值发生在外层作用域, 编不过。
//
// 因两个 id 共享槽派生的 varN 名、javac 按名字解析局部变量, 治本是把「已存在的同名声明」扩到能词法覆盖所有
// 该名字出现处的最低公共祖先块 (含 if/loop 条件这种 head 引用)。kill-switch JDEC_COVER_UNDECLARED_OFF=1
// 关掉该名字制安全网必复现这些 "cannot find symbol: variable varN"。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classUndeclaredVarErrors decompiles the named class entries from jarPath, recompiles them against
// the jar, and returns the count of "cannot find symbol: variable varN" javac errors (the undeclared
// generated-local residual). killOff toggles JDEC_COVER_UNDECLARED_OFF around the decompile so the
// caller can compare fix-ON vs fix-OFF.
func classUndeclaredVarErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_COVER_UNDECLARED_OFF"
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
	// javac prints the message on the line AFTER "cannot find symbol"; counting the symbol detail line
	// "symbol:   variable varN" isolates the undeclared-local residual from "method"/"class" not-found.
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "symbol:") && strings.Contains(s, "variable var") {
			n++
		}
	}
	return n
}

// TestCoverUndeclaredLocalIsLoadBearing pins the three fastjson2 split-slot undeclared-local residuals.
// With the fix ON every `cannot find symbol: variable varN` must be gone; disabling the name-based
// coverage pass via the kill-switch must reintroduce them.
func TestCoverUndeclaredLocalIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{
		"com/alibaba/fastjson2/writer/ObjectWriterAdapter.class",
		"com/alibaba/fastjson2/JSONReaderUTF16.class",
		"com/alibaba/fastjson2/JSONReaderUTF8.class",
	}

	on := classUndeclaredVarErrors(t, jarPath, entries, false) // fix ON
	off := classUndeclaredVarErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("undeclared variable varN errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all undeclared-local errors: ON=%d (want 0)", on)
	}
}
