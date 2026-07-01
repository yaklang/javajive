package cross

// 承重测试: 「内联 lambda 体在外围方法解析途中复用同一 FuncCtx, 解析完未还原导致外围方法上下文被污染」治本.
//
// 真实 fastjson2 BeanUtils.isWriteEnumAsJavaBean(Class):boolean 内含一个 invokedynamic lambda
// (`(Method l0) -> processJSONType1x(...)`, 其 impl 方法 `lambda$...$2` 返回 void)。lambda 体是在
// 外围方法做栈模拟、构造 invokedynamic 值时「就地懒解析」的: 递归 DumpMethodWithInitialId 把共享的
// dumper.FuncCtx 的 FunctionName/FunctionType 覆写成 lambda 的 (void), 解析完未还原。因 parser
// .FunctionContext 就是同一个 dumper.FuncCtx 指针, 外围方法位于 invokedynamic 之后的剩余字节码
// (尾部 `return false/true`, javac 编成 `iconst_0/1; ireturn`) 在 resetReturnValueTypeSafe 里就拿
// 到了 lambda 的 void 返回类型, int->boolean 重定型不触发, 于是渲染成 `return 1;`/`return 0;` 而非
// `return true;`/`return false;`, javac 报 "int cannot be converted to boolean"。治本: 在递归
// lambda dump 前后保存/还原外围方法的 per-method 上下文。JDEC_LAMBDA_CTX_RESTORE_OFF=1 关掉还原必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classLambdaCtxIntBoolErrors decompiles the named class entries from jarPath, recompiles them
// against the jar, and returns the count of "int cannot be converted to boolean" javac errors.
// killOff toggles JDEC_LAMBDA_CTX_RESTORE_OFF around the decompile so the caller can compare
// fix-ON vs fix-OFF.
func classLambdaCtxIntBoolErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_LAMBDA_CTX_RESTORE_OFF"
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
	return strings.Count(string(out), "int cannot be converted to boolean")
}

// TestLambdaCtxRestoreIsLoadBearing pins fastjson2 BeanUtils: a boolean method whose body contains
// an inline lambda BEFORE a trailing `return false/true` must restore the enclosing method's
// FuncCtx after the re-entrant lambda dump, so the trailing return resets against `boolean` and
// renders `return true/false` (not `return 1/0`). Disabling the restore via the kill-switch must
// reintroduce the "int cannot be converted to boolean" errors.
func TestLambdaCtxRestoreIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{"com/alibaba/fastjson2/util/BeanUtils.class"}

	on := classLambdaCtxIntBoolErrors(t, jarPath, entries, false) // fix ON
	off := classLambdaCtxIntBoolErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("BeanUtils int->boolean errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
}
