package cross

// 承重测试: 「switch 合并点的非支配前驱不应被改写成 break」治本 (CODEC_TODO §3 break-outside-loop)。
//
// 真实 fastjson2 ObjectReaderImplMap.of 在一个 switch 之前有 if/else-if 链, 链中各分支前向跳到与该
// switch 相同的合并点 (post-switch continuation)。旧逻辑给合并点的「所有」前驱都插入 break 叶子, 把这些
// 位于 switch 之外的 if 链边也变成裸 break, javac 报 "break outside switch or loop"。治本: 仅当前驱被
// switch 节点支配 (确属 switch 区域) 才插 break。JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classBreakOutsideErrors decompiles exactly the named class entries from jarPath, recompiles them
// against the jar, and returns the count of "break outside switch or loop" javac errors. killOff
// toggles JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF around the decompile so the caller can compare fix-ON
// vs fix-OFF on identical inputs.
func classBreakOutsideErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF")
	if killOff {
		os.Setenv("JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF", "1")
	} else {
		os.Unsetenv("JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF")
	}
	defer func() {
		if had {
			os.Setenv("JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF", prev)
		} else {
			os.Unsetenv("JDEC_SWITCH_NONDOM_MERGE_BREAK_OFF")
		}
	}()

	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	dir := t.TempDir()
	var files []string
	for _, entry := range entries {
		// JarFS.ReadFile decompiles the .class on read (honoring the kill-switch env var set above).
		src, err := jfs.ReadFile(entry)
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
	return strings.Count(string(out), "error: break outside switch or loop")
}

// TestSwitchNonDomMergeBreakIsLoadBearing pins fastjson2 ObjectReaderImplMap: the if-chain edges that
// share the trailing switch's merge point must NOT become bare `break`s. Disabling the fix via the
// kill-switch must reproduce the "break outside switch or loop" defect.
func TestSwitchNonDomMergeBreakIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	entries := []string{"com/alibaba/fastjson2/reader/ObjectReaderImplMap.class"}

	on := classBreakOutsideErrors(t, jarPath, entries, false) // fix ON
	off := classBreakOutsideErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("ObjectReaderImplMap break-outside-loop errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on != 0 {
		t.Errorf("fix did not eliminate break-outside-loop: ON=%d (expected 0)", on)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
}
