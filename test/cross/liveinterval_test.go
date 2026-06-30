package cross

// Phase 1 承重测试 (Bug AL: 局部变量活跃区间 / 声明摆放)。
//
// 它从真实 fastjson2 jar 取 JdbcSupport 类组 (含内部类 JdbcSupport$TimeReader), 用生产路径反编译
// 后整组重编译, 断言「修复 ON 的 javac 错误数 严格少于 修复 OFF」——即 JDEC_LIVEINTERVAL_OFF 关掉
// 治本后必能复现缺陷 (load-bearing)。承载 CODEC_TODO 的 fastjson2 多槽复用主导杠杆。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// groupRecompileErrors decompiles every class in jarPath whose entry name starts with classPrefix
// (an Outer + its Outer$* inner units), recompiles the whole group against the jar, and returns the
// javac error count. killOff toggles JDEC_LIVEINTERVAL_OFF around the decompile so the caller can
// compare fix-ON vs fix-OFF on the identical inputs.
func groupRecompileErrors(t *testing.T, jarPath, classPrefix string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_LIVEINTERVAL_OFF")
	if killOff {
		os.Setenv("JDEC_LIVEINTERVAL_OFF", "1")
	} else {
		os.Unsetenv("JDEC_LIVEINTERVAL_OFF")
	}
	defer func() {
		if had {
			os.Setenv("JDEC_LIVEINTERVAL_OFF", prev)
		} else {
			os.Unsetenv("JDEC_LIVEINTERVAL_OFF")
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

// TestLiveIntervalSplitIsLoadBearing pins the fastjson2 JdbcSupport group: the live-interval
// declaration placement must strictly reduce its recompile errors, and disabling it via the
// kill-switch must reproduce the defect.
func TestLiveIntervalSplitIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const prefix = "com/alibaba/fastjson2/util/JdbcSupport"

	on := groupRecompileErrors(t, jarPath, prefix, false) // fix ON
	off := groupRecompileErrors(t, jarPath, prefix, true) // fix OFF (kill-switch)
	t.Logf("JdbcSupport group recompile errors: ON=%d OFF=%d", on, off)

	if off <= on {
		t.Errorf("live-interval fix is NOT load-bearing: ON=%d OFF=%d (OFF must reproduce more errors)",
			on, off)
	}
	if on >= off {
		t.Errorf("live-interval fix did not reduce errors: ON=%d OFF=%d", on, off)
	}
}
