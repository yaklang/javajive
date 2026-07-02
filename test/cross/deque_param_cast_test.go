package cross

// 承重测试:「Deque<E> 头尾单元素插入方法(addFirst/addLast/offerFirst/offerLast/push)的 E 形参缺实参造型,
// JDK 形参表未覆盖故 arg-cast 不触发」治本 (CODEC_TODO 泛型擦除·Deque 插入形参表)。
//
// 真实 guava Iterators$ConcatenatedIterator<T>:
//   Deque<Iterator<? extends Iterator<? extends T>>> metaIterators;
//   this.metaIterators.addFirst(var1.metaIterators.removeLast());
// var1 为裸 raw ConcatenatedIterator, 故 var1.metaIterators 为裸 Deque、removeLast() 返回 Object;
// this.metaIterators 经字段签名还原为 Deque<Iterator<? extends Iterator<? extends T>>>, addFirst 形参真类型
// 是 Iterator<? extends Iterator<? extends T>>, 但 jdkMethodParamTypeArgIndex 原只覆盖 Collection.add/offer,
// 不含 Deque.addFirst 等, 故形参未还原、不补造型, javac 判 `Object cannot be converted to
// Iterator<? extends Iterator<? extends T>>`(源码原带造型)。治法: 新 jdkDequeFamily + 在
// jdkMethodParamTypeArgIndex 增 Deque 头尾插入方法(形参 0=E), 既有参数化实参造型路径 resolvedParameterizedArgCast
// 遂补 `(Iterator<...>)` 造型。JDEC_DEQUE_PARAM_OFF=1 关掉 Deque 形参表必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// dequeParamCastErrCount decompiles ConcatenatedIterator under the kill-switch, recompiles it against
// the jar, and counts the deque-insertion defect: an Object argument fed to Deque.addFirst without the
// `(Iterator<...>)` cast, so javac rejects "cannot be converted to Iterator<? extends Iterator<? extends
// T>>". The substring isolates precisely this defect.
func dequeParamCastErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_DEQUE_PARAM_OFF"
	const entry = "com/google/common/collect/Iterators$ConcatenatedIterator.class"
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
	dst := filepath.Join(dir, "Iterators$ConcatenatedIterator.java")
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}

	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", jarPath, "-d", t.TempDir(), dst)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ": error:") {
			continue
		}
		if strings.Contains(line, "cannot be converted to Iterator<? extends Iterator<? extends T>>") {
			n++
		}
	}
	return n
}

// TestDequeParamCastIsLoadBearing pins guava Iterators$ConcatenatedIterator: the Object argument fed to
// `this.metaIterators.addFirst(...)` (metaIterators is Deque<Iterator<? extends Iterator<? extends T>>>)
// must be wrapped in the element cast. Disabling the Deque param table must reintroduce the error.
func TestDequeParamCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := dequeParamCastErrCount(t, jarPath, false) // fix ON
	off := dequeParamCastErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("ConcatenatedIterator deque-param-cast errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the deque-param-cast error: ON=%d (want 0)", on)
	}
}
