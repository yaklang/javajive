package cross

// 承重测试: 「synchronized 块本体本身就是一个 try/catch 时, javac 合成的正常路径 monitorexit 会被结构化
// 下沉进内层 try 的 body 末尾, 致 SynchronizeRewriter 的顶层 monitorexit 扫描漏掉、发出空 synchronized 块,
// 把内层 try/catch + return 整块丢弃」治本.
//
// 真实 gson 残留: JsonStreamParser.hasNext
//   public boolean hasNext(){ synchronized(lock){ try { return parser.peek()!=END_DOCUMENT; }
//                                                  catch(MalformedJsonException e){throw new JsonSyntaxException(e);}
//                                                  catch(IOException e){throw new JsonIOException(e);} } }
// monitorexit(offset 26) 被吸收进内层 try-body, 外层「synchronized 包装 try」的 TryBody 只剩内层
// TryCatchStatement 一个元素, 顶层扫不到 monitor_exit → bodySts 留空 → 发出空 synchronized 块, 方法无 return
// → javac "missing return statement"。
//
// 治本(removeSunkMonitorExit)在顶层扫不到 monitorexit 时, DFS 下潜嵌套 try-body 就地删除被下沉的正常路径
// monitorexit, 并把整个 TryBody(含内层 try/catch/return)作为 synchronized body。
// kill-switch JDEC_SYNC_NESTED_MONITOREXIT_OFF=1 关掉该回退必复现 "missing return statement"。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// classMissingReturnErrors decompiles the named class entries, recompiles them against the jar, and
// returns the count of "missing return statement" errors. killOff toggles
// JDEC_SYNC_NESTED_MONITOREXIT_OFF around the decompile so the caller can compare fix-ON vs fix-OFF.
func classMissingReturnErrors(t *testing.T, jarPath string, entries []string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_SYNC_NESTED_MONITOREXIT_OFF"
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
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "missing return statement") {
			n++
		}
	}
	return n
}

// TestSyncNestedMonitorExitIsLoadBearing pins the gson JsonStreamParser.hasNext synchronized-body-loss
// residual. With the fix ON the synchronized body (try/catch/return) is reconstructed so the method
// returns; disabling the nested-monitorexit fallback via the kill-switch must reintroduce the
// "missing return statement".
func TestSyncNestedMonitorExitIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["gson"].relPath)
	if jarPath == "" {
		t.Skip("gson jar not found under ~/.m2; skipping")
	}
	entries := []string{
		"com/google/gson/JsonStreamParser.class",
	}

	on := classMissingReturnErrors(t, jarPath, entries, false) // fix ON
	off := classMissingReturnErrors(t, jarPath, entries, true) // fix OFF (kill-switch)
	t.Logf("missing-return-statement errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all missing-return errors: ON=%d (want 0)", on)
	}
}
