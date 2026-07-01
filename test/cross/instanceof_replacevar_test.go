package cross

// 承重测试: 「instanceof 操作数被 OP_INSTANCEOF 捕获进 CustomValue 闭包, 但该 CustomValue 漏传 replaceFunc,
// 于是 rewriteVar 末期对某槽不相交复用所做的 ReplaceVar(oldId->newId) 够不到 instanceof 操作数」治本.
//
// 真实 fastjson2 残留: JSONSchema.of
//   `var6_3 = var0.get("exclusiveMaximum"); ... if (!(var6 instanceof Integer) ...)`
// 同一 JVM 槽在 if/else 不相交臂里先后承载 int(数组长度)与 Object(get 结果), rewriteVar 正确地为
// 二者各 mint 一个身份, store 端被 ReplaceVar 重绑成 Object 的 var6_3, 但 instanceof 操作数因 CustomValue
// 漏传 replaceFunc 而冻结在 int 身份, 渲染成 `var6 instanceof Integer`, javac 报
// "unexpected type / required: reference / found: int"(JSONSchema.of 共 9 行)。
//
// 治本: OP_INSTANCEOF 的 CustomValue 像 OP_CHECKCAST 与数值转换那样转发 ReplaceVar 到捕获的操作数。
// kill-switch JDEC_INSTANCEOF_REPLACEVAR_OFF=1 关掉转发必复现这些 "unexpected type" 错误。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classUnexpectedTypeErrors decompiles the named class entries, recompiles them against the jar, and
// returns the count of "unexpected type" errors whose found-type is a primitive (e.g. `int instanceof
// X`). killOff toggles JDEC_INSTANCEOF_REPLACEVAR_OFF around the decompile so the caller can compare
// fix-ON vs fix-OFF.
func classUnexpectedTypeErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_INSTANCEOF_REPLACEVAR_OFF"
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
	// `int instanceof X` makes javac print "error: unexpected type" then a "found: <primitive>" line.
	// Counting the error headers is enough to track this fix's effect.
	return strings.Count(string(out), "error: unexpected type")
}

// TestInstanceofReplaceVarIsLoadBearing pins the fastjson2 JSONSchema.of instanceof-operand residual.
// With the fix ON every `int instanceof X` "unexpected type" must be gone; disabling the ReplaceVar
// forwarding via the kill-switch must reintroduce them.
func TestInstanceofReplaceVarIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{
		"com/alibaba/fastjson2/schema/JSONSchema.class",
	}

	on := classUnexpectedTypeErrors(t, jarPath, entries, false) // fix ON
	off := classUnexpectedTypeErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("'unexpected type' (primitive instanceof) errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all primitive-instanceof errors: ON=%d (want 0)", on)
	}
}
