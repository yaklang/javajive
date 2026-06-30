package cross

// 承重测试: 「未被使用的 lambda 形参应隐式渲染(去掉显式类型)」治本 (CODEC_TODO §3 incompatible-param-lambda)。
//
// 真实 fastjson2 ObjectReaderCreatorASM 多处 `rawMap.computeIfAbsent(k, (Integer l0) -> new ArrayList())`:
// 接收者是裸类型 Map, 其 computeIfAbsent 的 SAM 退化为 raw Function(apply(Object)), 而 lambda 形参带显式
// Integer 类型, javac 报 "incompatible parameter types in lambda expression"。由于 lambda 体根本没用到该
// 形参, 去掉显式类型(隐式 `(l0) -> ...`)既更贴近源码也让 javac 从目标类型推断从而通过编译。
// JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classLambdaParamErrors decompiles the named class entries from jarPath, recompiles them against the
// jar, and returns the count of "incompatible parameter types in lambda expression" javac errors.
// killOff toggles JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF around the decompile so the caller can compare
// fix-ON vs fix-OFF on identical inputs.
func classLambdaParamErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF")
	if killOff {
		os.Setenv("JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF", "1")
	} else {
		os.Unsetenv("JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF")
	}
	defer func() {
		if had {
			os.Setenv("JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF", prev)
		} else {
			os.Unsetenv("JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF")
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
	return strings.Count(string(out), "error: incompatible types: incompatible parameter types in lambda expression")
}

// TestLambdaImplicitUnusedParamIsLoadBearing pins fastjson2 ObjectReaderCreatorASM: its
// `computeIfAbsent(k, (Integer l0) -> new ArrayList())` lambdas (param unused) must render with
// implicit parameters so they bind to the raw functional-interface target. Disabling the fix via the
// kill-switch must reintroduce "incompatible parameter types in lambda expression".
func TestLambdaImplicitUnusedParamIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{"com/alibaba/fastjson2/reader/ObjectReaderCreatorASM.class"}

	on := classLambdaParamErrors(t, jarPath, entries, false) // fix ON
	off := classLambdaParamErrors(t, jarPath, entries, true)  // fix OFF (kill-switch)
	t.Logf("ObjectReaderCreatorASM lambda-param errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
}
