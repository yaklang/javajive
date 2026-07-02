package cross

// 承重测试:「跨类静态字段单例返回缺 raw 桥接造型」治本 (CODEC_TODO 泛型擦除/返回造型族).
//
// 真实 guava Range.rangeLexOrdering:
//   static <C extends Comparable<?>> Ordering<Range<C>> rangeLexOrdering() {
//       return (Ordering<Range<C>>) (Ordering) Range.RangeLexOrdering.INSTANCE;   // 源码带 raw (Ordering) 桥接
//   }
// 字段 INSTANCE 声明在 Range$RangeLexOrdering 里 (类型 Ordering<Range<?>>), 是跨类静态字段。反编译器读它
// 只见描述符擦除的裸 raw Ordering, 从值类型看 (Ordering<Range<C>>) rawOrdering 似 unchecked-legal、不需桥接,
// 遂 typeVarReturnCast 只补直接造型 (Ordering<Range<C>>); javac 却按 INSTANCE 真实声明泛型 Ordering<Range<?>>
// 定型, 判 (Ordering<Range<C>>)(Ordering<Range<?>>) inconvertible -> "Ordering<Range<?>> cannot be converted
// to Ordering<Range<C>>"。治本 (nestedGenericRawBridge Case B 扩展: 跨类静态字段经 SiblingFieldSig 恢复声明
// 泛型, 判同擦除异参且顶层非裸通配后发 raw 桥接 (Ordering<Range<C>>)(Ordering)(INSTANCE))。
// JDEC_XCLASS_FIELD_RET_BRIDGE_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// xclassFieldRetBridgeErrCount decompiles Range under the kill-switch, recompiles it against the jar,
// and returns the count of the line-27-specific inconvertible-cast error. The file has other unrelated
// errors (closed-arg erasure at :98/:114, create-arg at :202) present under both switch states; the
// exact substring "Ordering<Range<?>> cannot be converted to Ordering<Range<C>>" isolates precisely the
// cross-class field singleton missing its raw bridge.
func xclassFieldRetBridgeErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_XCLASS_FIELD_RET_BRIDGE_OFF"
	const entry = "com/google/common/collect/Range.class"
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
	dst := filepath.Join(dir, "Range.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "Range.java") {
			continue
		}
		// The cross-class INSTANCE (declared Ordering<Range<?>>) cast directly to the method's
		// Ordering<Range<C>> return without the raw bridge.
		if strings.Contains(line, "Ordering<Range<?>> cannot be converted to Ordering<Range<C>>") {
			n++
		}
	}
	return n
}

// TestXClassFieldRetBridgeIsLoadBearing pins guava Range.rangeLexOrdering: a cross-class static field
// singleton (`Range$RangeLexOrdering.INSTANCE`, declared `Ordering<Range<?>>`) returned as
// `Ordering<Range<C>>` must be bridged via the raw type (`(Ordering<Range<C>>) (Ordering) INSTANCE`),
// its declared parameterization recovered through SiblingFieldSig. Disabling the fix via the kill-switch
// must reintroduce the inconvertible direct-cast error.
func TestXClassFieldRetBridgeIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := xclassFieldRetBridgeErrCount(t, jarPath, false) // fix ON
	off := xclassFieldRetBridgeErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("Range.rangeLexOrdering cross-class field singleton bridge errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the cross-class field bridge error: ON=%d (want 0)", on)
	}
}
