package cross

// 承重测试:「泛型方法尾部变参型变数组形参 `E...` 收到裸 Object[] 缺 (E[]) 造型」治本
// (CODEC_TODO 泛型擦除·变参型变数组实参造型)。
//
// 真实 guava ImmutableSortedSet.construct 声明:
//   static <E> ImmutableSortedSet<E> construct(Comparator<? super E> var0, int var1, E... var2)
// 两处调用把 Object[] 直接喂给变参 E... :
//   copyOf :84   construct(var0, var2_1.length, var2_1)              // var2_1 = Object[]
//   Builder:36   construct(this.comparator, this.size, (Object[])this.contents)
// 无 (E[]) 造型时 javac 对 E 同时约束为 Object(来自数组实参) 与调用者 E(来自 Comparator<? super E> 见证),
// 判 "method construct in class ImmutableSortedSet<E> cannot be applied to given types"。治本 (新
// varargsTypeVarArrayArgParamType: 尾部形参为方法型变数组 E[]、实参为异元素引用数组时, 经见证形参
// Comparator<? super E>(含 ? super/extends 通配, 且对裸字段读回退 RecoverThisFieldInstantiatedType)
// 把 E 钉到调用者作用域内型变, 补 (E[]) unchecked 造型; 把 Object[] 造到「见证已固定的调用者 E[]」永远合法)。
// JDEC_VARARGS_TYPEVAR_ARRAY_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// varargsTypeVarArrayErrCount decompiles the two ImmutableSortedSet units under the kill-switch,
// recompiles them against the jar, and counts the varargs-typevar-array defect: the `Object[]` fed to
// the `E...` varargs formal of `construct` without the `(E[])` cast, so javac rejects
// "method construct in class ImmutableSortedSet<E> cannot be applied to given types". The substring
// isolates precisely this defect (each file's other errors are flattened-`$` symbol lookups).
func varargsTypeVarArrayErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_VARARGS_TYPEVAR_ARRAY_OFF"
	entries := map[string]string{
		"com/google/common/collect/ImmutableSortedSet.class":         "ImmutableSortedSet.java",
		"com/google/common/collect/ImmutableSortedSet$Builder.class": "ImmutableSortedSet$Builder.java",
	}
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
	for entry, base := range entries {
		src, err := jfs.ReadFile(entry) // JarFS.ReadFile decompiles on read (honoring the kill-switch).
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		dst := filepath.Join(dir, base)
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
		if !strings.Contains(line, ": error:") {
			continue
		}
		if strings.Contains(line, "method construct in class ImmutableSortedSet") &&
			strings.Contains(line, "cannot be applied") {
			n++
		}
	}
	return n
}

// TestVarargsTypeVarArrayCastIsLoadBearing pins guava ImmutableSortedSet.construct's two call sites: the
// bare `Object[]` passed to the `E...` varargs formal must be wrapped in an `(E[])` cast. Disabling the
// fix via the kill-switch must reintroduce the "method construct ... cannot be applied" error.
func TestVarargsTypeVarArrayCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := varargsTypeVarArrayErrCount(t, jarPath, false) // fix ON
	off := varargsTypeVarArrayErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("ImmutableSortedSet.construct varargs-typevar-array errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the varargs-typevar-array error: ON=%d (want 0)", on)
	}
}
