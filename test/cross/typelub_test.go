package cross

// Phase 2 承重测试 (Bug AL LUB 子族: 类型层级 / 三元最小上界 / 声明摆放类型)。
//
// 它从真实 commons-codec jar 取 DaitchMokotoffSoundex 类组 (含内部类 $Branch / $Rule), 用生产路径
// 反编译后整组重编译, 断言「修复 ON 的 javac 错误数 严格少于 修复 OFF」——即 JDEC_TERNARY_DECL_LUB_OFF
// 关掉治本后必能复现缺陷 (load-bearing)。根因: `ArrayList x = b ? new ArrayList() : Collections.emptyList()`
// 的声明槽在 DFS 期按首臂 (ArrayList) 定型, 而控制流汇点的真实类型是双臂最小上界 (List), 窄声明拒绝
// LUB 三元右值 ("incompatible types: bad type in conditional expression")。治本把声明上提到 LUB。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// groupRecompileErrorsSwitch is groupRecompileErrors parameterized by the kill-switch env var, so a
// Phase-specific load-bearing test can A/B its own fix on an identical decompile group.
func groupRecompileErrorsSwitch(t *testing.T, jarPath, classPrefix, envVar string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv(envVar)
	if killOff {
		os.Setenv(envVar, "1")
	} else {
		os.Unsetenv(envVar)
	}
	defer func() {
		if had {
			os.Setenv(envVar, prev)
		} else {
			os.Unsetenv(envVar)
		}
	}()

	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	dir := t.TempDir()
	var files []string
	for _, entry := range classEntries(t, jarPath) {
		if !strings.HasPrefix(entry, classPrefix) {
			continue
		}
		raw, err := jfs.ReadFile(entry)
		if err != nil {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(entry), ".class") // keep flat Outer$Inner name
		dst := filepath.Join(dir, base+".java")
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		files = append(files, dst)
	}
	if len(files) == 0 {
		t.Fatalf("no classes matched prefix %q in %s", classPrefix, jarPath)
	}

	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", jarPath, "-d", t.TempDir())
	args = append(args, files...)
	out, _ := exec.Command(javac, args...).CombinedOutput()
	return strings.Count(string(out), ": error:")
}

// TestTypeLUBIsLoadBearing pins the commons-codec DaitchMokotoffSoundex group: widening a ternary
// declaration to its arm least-upper-bound must strictly reduce the group's recompile errors, and
// disabling it via JDEC_TERNARY_DECL_LUB_OFF must reproduce the defect.
func TestTypeLUBIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["codec"].relPath)
	if jarPath == "" {
		t.Skip("commons-codec jar not found under ~/.m2; skipping")
	}
	const prefix = "org/apache/commons/codec/language/DaitchMokotoffSoundex"

	on := groupRecompileErrorsSwitch(t, jarPath, prefix, "JDEC_TERNARY_DECL_LUB_OFF", false) // fix ON
	off := groupRecompileErrorsSwitch(t, jarPath, prefix, "JDEC_TERNARY_DECL_LUB_OFF", true) // fix OFF
	t.Logf("DaitchMokotoffSoundex group recompile errors: ON=%d OFF=%d", on, off)

	if off <= on {
		t.Errorf("ternary-decl-LUB fix is NOT load-bearing: ON=%d OFF=%d (OFF must reproduce more errors)",
			on, off)
	}
}
