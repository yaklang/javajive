package cross

// 承重测试:「继承字段泛型 Signature 丢失致 this.<继承字段>.方法(...) 无法恢复实参造型」治本 (CODEC_TODO 泛型擦除族).
//
// 真实 guava RegularContiguousSet<C extends Comparable> extends ContiguousSet<C>:
//   int indexOf(Object var1) {
//       return this.contains(var1) ? (int) this.domain.distance(this.first(), (C) var1) : -1;
//   }
// 字段 domain 声明在超类 ContiguousSet 里 (类型 DiscreteDomain<C>),不在当前类的 FieldSignatures 中,
// 故 this.domain 退化成裸 raw DiscreteDomain,distance(C,C) 的形参 C 无法被 receiverParamTypeArgs 恢复,
// 于是 var1 的源码 (C) 造型 (字节码擦成 checkcast Comparable) 渲染成 (Comparable),javac 报
// "incompatible types: Comparable cannot be converted to C"。治本 (SiblingFieldSig 提供各类字段泛型
// Signature + types.ResolveInstantiatedFieldType 沿类层级带类型实参代换恢复继承字段实时类型 ->
// DiscreteDomain<C>,resolvedParamType 遂解析出 distance 形参 C,renderArgAt 重下 (C) 造型)。
// JDEC_INHERITED_FIELD_SIG_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// inheritedFieldSigErrCount decompiles RegularContiguousSet under the kill-switch, recompiles it against
// the jar, and returns the count of the inherited-field-loss recompile error at indexOf: the source's
// `(C) var1` argument to `this.domain.distance(C, C)` renders `(Comparable)` (the erased checkcast)
// when the inherited `domain` field's `DiscreteDomain<C>` type is lost, so javac rejects
// "Comparable cannot be converted to C". The substring is exact enough to isolate precisely this defect.
func inheritedFieldSigErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_INHERITED_FIELD_SIG_OFF"
	const entry = "com/google/common/collect/RegularContiguousSet.class"
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
	dst := filepath.Join(dir, "RegularContiguousSet.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "RegularContiguousSet.java") {
			continue
		}
		// The `(C) var1` erased to `(Comparable)`: the `distance(C, C)` param cannot take a Comparable.
		if strings.Contains(line, "Comparable cannot be converted to C") {
			n++
		}
	}
	return n
}

// TestInheritedFieldSigIsLoadBearing pins guava RegularContiguousSet: an inherited parameterized field
// (`this.domain`, declared `DiscreteDomain<C>` in the superclass ContiguousSet) must have its generic
// Signature recovered by the cross-class hierarchy walk so `this.domain.distance(first(), var1)` can
// re-emit the erased `(C)` cast on var1. Disabling the fix via the kill-switch must reintroduce the
// "Comparable cannot be converted to C" error.
func TestInheritedFieldSigIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := inheritedFieldSigErrCount(t, jarPath, false) // fix ON
	off := inheritedFieldSigErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("RegularContiguousSet inherited-field (this.domain) arg-cast errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the inherited-field arg-cast error: ON=%d (want 0)", on)
	}
}
