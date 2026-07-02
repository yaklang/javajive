package cross

// 承重测试:「继承字段直接返回、字段真实通配泛型被返回目标钉成具体参数化, 缺受检造型」治本 (CODEC_TODO 泛型擦除/返回造型族).
//
// 真实 guava RegularImmutableSortedSet<E extends Object> extends ImmutableSortedSet<E>:
//   Comparator<Object> unsafeComparator() {
//       return (Comparator<Object>) this.comparator;   // 源码带受检造型
//   }
// 字段 comparator 声明在超类 ImmutableSortedSet 里 (类型 Comparator<? super E>), 是继承字段。反编译器读它
// 只见描述符擦除的裸 raw Comparator, `return this.comparator` 不补造型; javac 却按字段真实声明泛型
// Comparator<? super E> 定型, 判 Comparator<CAP#1> 无法转 Comparator<Object> ->
// "Comparator<CAP#1> cannot be converted to Comparator<Object>"。治本 (inheritedFieldReturnCast 经
// values.RecoverThisFieldInstantiatedType 沿类层级恢复继承字段真实类型 Comparator<? super E>, 判与返回
// 目标同擦除、源含顶层通配后, 补 unchecked (Comparator<Object>) 返回造型)。
// JDEC_INHERITED_FIELD_RET_CAST_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// inheritedFieldRetCastErrCount decompiles RegularImmutableSortedSet under the kill-switch, recompiles
// it against the jar, and returns the count of the inherited-field return-cast defect: the returned
// inherited field `this.comparator` (declared `Comparator<? super E>` in the superclass) rendered
// without the wrapping `(Comparator<Object>)` cast, so javac rejects
// "Comparator<CAP#1> cannot be converted to Comparator<Object>". The substring isolates precisely this
// defect (the file's only other errors, if any, never carry it).
func inheritedFieldRetCastErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_INHERITED_FIELD_RET_CAST_OFF"
	const entry = "com/google/common/collect/RegularImmutableSortedSet.class"
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
	dst := filepath.Join(dir, "RegularImmutableSortedSet.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "RegularImmutableSortedSet.java") {
			continue
		}
		// The inherited wildcard-parameterized field returned without the (Comparator<Object>) cast.
		if strings.Contains(line, "cannot be converted to Comparator<Object>") {
			n++
		}
	}
	return n
}

// TestInheritedFieldRetCastIsLoadBearing pins guava RegularImmutableSortedSet.unsafeComparator: an
// inherited wildcard-parameterized field (`this.comparator`, declared `Comparator<? super E>` in the
// superclass) returned as the concrete `Comparator<Object>` must be wrapped in an unchecked
// `(Comparator<Object>)` cast, its real type recovered through the cross-class hierarchy walk. Disabling
// the fix via the kill-switch must reintroduce the "cannot be converted to Comparator<Object>" error.
func TestInheritedFieldRetCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := inheritedFieldRetCastErrCount(t, jarPath, false) // fix ON
	off := inheritedFieldRetCastErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("RegularImmutableSortedSet inherited-field return-cast errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the inherited-field return-cast error: ON=%d (want 0)", on)
	}
}
