package cross

// 承重测试:「同类具体参数化字段被赋以裸渲染的调用值(真实实例化返回是同擦除通配), 缺受检造型」治本
// (CODEC_TODO 泛型擦除/返回·字段造型族)。
//
// 真实 guava ImmutableSortedMap$SerializedForm 构造器:
//   ImmutableSortedMap$SerializedForm(ImmutableSortedMap<?, ?> var1) {
//       super(var1);
//       this.comparator = (Comparator<Object>) var1.comparator();   // 源码带受检造型
//   }
// var1 是 ImmutableSortedMap<?,?>, ImmutableSortedMap.comparator() 真实返回 Comparator<? super K>
// (捕获为 Comparator<CAP#1>)。反编译器读该调用只见描述符擦除的裸 raw Comparator, `this.comparator =
// var1.comparator()` 不补造型; javac 却按被调方真实泛型返回定型, 判 Comparator<CAP#1> 无法转字段
// Comparator<Object> -> "Comparator<CAP#1> cannot be converted to Comparator<Object>"。治本
// (wildcardReturnFieldStoreCast 经 types.ResolveInstantiatedReturnType 沿类层级恢复调用真实实例化返回
// Comparator<? super K>, 判与字段同擦除、源含顶层通配后, 补 unchecked (Comparator<Object>) 字段存值造型)。
// JDEC_WILDCARD_RETURN_FIELD_CAST_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// wildcardReturnFieldStoreErrCount decompiles ImmutableSortedMap$SerializedForm under the kill-switch,
// recompiles it against the jar, and counts the field-store cast defect: the raw-rendered call value
// `var1.comparator()` (whose real instantiated return is `Comparator<? super K>`) stored into the
// concrete `Comparator<Object>` field without the wrapping `(Comparator<Object>)` cast, so javac rejects
// "Comparator<CAP#1> cannot be converted to Comparator<Object>". The substring isolates precisely this
// defect (the file's other errors are flattened-`$` symbol lookups that never carry it).
func wildcardReturnFieldStoreErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_WILDCARD_RETURN_FIELD_CAST_OFF"
	const entry = "com/google/common/collect/ImmutableSortedMap$SerializedForm.class"
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
	dst := filepath.Join(dir, "ImmutableSortedMap$SerializedForm.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "ImmutableSortedMap$SerializedForm.java") {
			continue
		}
		if strings.Contains(line, "cannot be converted to Comparator<Object>") {
			n++
		}
	}
	return n
}

// TestWildcardReturnFieldStoreCastIsLoadBearing pins guava ImmutableSortedMap$SerializedForm's ctor: a
// raw-rendered call value whose real instantiated return is a wildcard parameterization
// (`var1.comparator()` -> `Comparator<? super K>`) stored into the concrete `Comparator<Object>` field
// must be wrapped in an unchecked `(Comparator<Object>)` cast, its real return recovered via the
// cross-class hierarchy walk. Disabling the fix via the kill-switch must reintroduce the "cannot be
// converted to Comparator<Object>" error.
func TestWildcardReturnFieldStoreCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := wildcardReturnFieldStoreErrCount(t, jarPath, false) // fix ON
	off := wildcardReturnFieldStoreErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("ImmutableSortedMap$SerializedForm wildcard-return field-store errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the wildcard-return field-store error: ON=%d (want 0)", on)
	}
}
