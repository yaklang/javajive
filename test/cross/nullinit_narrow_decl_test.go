package cross

// 承重测试: 「孤儿生成局部变量(被赋值/读取却没有声明)被 dumper 的文本兜底网默认成 `Object varN = null`,
// 于是 `varN.method(...)` 因接收者类型 Object 解析不到方法而编不过」治本.
//
// 真实 fastjson2 残留: ObjectWriterAdapter.writeWithFilter
//   `var30 = fieldWriter.getObjectWriter(...); var30.write(...)`
// var30 在 RewriteVar 末期没有任何声明(跨作用域 hoist 把它的声明丢了), dumper 文本兜底
// (addMissingGeneratedLocalDecls) 因为 `getObjectWriter` 是跨类调用、文本无法解析返回类型, 只能默认
// `Object var30 = null`, 三处 `var30.write(...)` 全部 "cannot find symbol: method write,
// location: variable var30 of type Object"。
//
// 治本(narrowNullInitObjectDecl)用 AST 里已解析好的 reassignment 类型 S=ObjectWriter, 在方法首部合成
// 正确类型的 `ObjectWriter var30;`(并仅当 varN 只作接收者/赋值目标使用时才动手, 保证不改变重载选择)。
// kill-switch JDEC_NULLINIT_NARROW_OFF=1 关掉该 pass 必复现这些 "of type Object" 接收者错误。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classObjectReceiverErrors decompiles the named class entries, recompiles them against the jar, and
// returns the count of "cannot find symbol" errors whose location is a "variable varN ... of type
// Object" — i.e. a member access on a local mistakenly declared Object. killOff toggles
// JDEC_NULLINIT_NARROW_OFF around the decompile so the caller can compare fix-ON vs fix-OFF.
func classObjectReceiverErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	// Isolate the nullinit-narrow fix from a LATER, more fundamental fix that also clears this sample:
	// the split-continue latch reconstruction (gson JsonWriter.string) repairs ObjectWriterAdapter's
	// dropped continue upstream, so its orphan locals get real declarations and the Object-receiver
	// residual never forms (OFF 3->0), masking this kill-switch (ON=0 OFF=0). Hold split-continue OFF
	// for BOTH measurements so the toggle below isolates nullinit-narrow alone (ON=0 OFF=3). Remove
	// once an Object-receiver residual exclusive to nullinit-narrow is pinned.
	prevSplit, hadSplit := os.LookupEnv("JDEC_SPLIT_CONTINUE_LATCH_OFF")
	os.Setenv("JDEC_SPLIT_CONTINUE_LATCH_OFF", "1")
	defer func() {
		if hadSplit {
			os.Setenv("JDEC_SPLIT_CONTINUE_LATCH_OFF", prevSplit)
		} else {
			os.Unsetenv("JDEC_SPLIT_CONTINUE_LATCH_OFF")
		}
	}()
	const sw = "JDEC_NULLINIT_NARROW_OFF"
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
	// javac prints the binding site on the "location:" line after a "cannot find symbol" error; a
	// "variable varN ... of type Object" location is exactly the Object-receiver residual this pass fixes.
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "location:") && strings.Contains(s, "variable var") && strings.HasSuffix(s, "of type Object") {
			n++
		}
	}
	return n
}

// TestNullInitNarrowDeclIsLoadBearing pins the fastjson2 ObjectWriterAdapter orphan-local residual.
// With the fix ON every Object-receiver "cannot find symbol" must be gone; disabling the narrowing
// pass via the kill-switch must reintroduce them.
func TestNullInitNarrowDeclIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{
		"com/alibaba/fastjson2/writer/ObjectWriterAdapter.class",
	}

	on := classObjectReceiverErrors(t, jarPath, entries, false) // fix ON
	off := classObjectReceiverErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("Object-receiver cannot-find-symbol errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all Object-receiver errors: ON=%d (want 0)", on)
	}
}
