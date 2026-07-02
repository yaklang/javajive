package cross

// 承重测试:「泛型解析器还原出的形参是参数化类型 `X<C>`(如 NavigableMap<Cut<C>,Range<C>>.tailMap 的键形参),
// 实参擦除异类故 arg-cast 漏 `(X<C>)` 造型」治本 (CODEC_TODO 泛型擦除·参数化实参造型)。
//
// 真实 guava TreeRangeSet 内部类:
//   RangesByUpperBound.entryIterator:74           this.rangesByLowerBound.tailMap(var2.getKey(), true)
//   SubRangeSetRangesByLowerBound.descendingEntryIterator:95  this.rangesByLowerBound.headMap(var1.endpoint(), ...)
// 字段 rangesByLowerBound 为 `NavigableMap<Cut<C>, Range<C>>`; tailMap/headMap 的键形参真类型是 `Cut<C>`,
// 但字节码擦成 Object、实参(`var2.getKey()`=Object / `var1.endpoint()`=Comparable)擦除异类故不补造型,
// javac 判 `Object/Comparable cannot be converted to Cut<C>`(源码原带 `(Cut<C>)` 造型)。
// 根因: 既有 arg-cast 类对类分支要求形参 `argType.RawType()` 是 *JavaClass(裸型变 K 才满足), 而解析器
// 还原出的 `Cut<C>` 是参数化类型、RawType() 非 *JavaClass 故 ok1=false、分支不触发、造型被丢。
// 治法: 新 `resolvedParameterizedArgCast` —— 解析器还原出参数化形参 `X<...>`、其 raw 类既非 Object 又与实参
// 擦除类不同、且实参非在域裸型变时补 `(X<...>)` unchecked 造型。
// JDEC_PARAM_ARG_CAST_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// parameterizedArgCastErrCount decompiles the two TreeRangeSet units under the kill-switch, recompiles
// them against the jar, and counts the parameterized-arg defect: an Object/Comparable-typed key argument
// fed to a NavigableMap<Cut<C>,Range<C>> navigation method without the `(Cut<C>)` cast, so javac rejects
// "cannot be converted to Cut<C>". The substring isolates precisely this defect (the units' other errors
// are flattened-`$` symbol lookups that never carry it).
func parameterizedArgCastErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_PARAM_ARG_CAST_OFF"
	entries := map[string]string{
		"com/google/common/collect/TreeRangeSet$RangesByUpperBound.class":            "TreeRangeSet$RangesByUpperBound.java",
		"com/google/common/collect/TreeRangeSet$SubRangeSetRangesByLowerBound.class": "TreeRangeSet$SubRangeSetRangesByLowerBound.java",
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
		if strings.Contains(line, "cannot be converted to Cut<C>") {
			n++
		}
	}
	return n
}

// TestParameterizedArgCastIsLoadBearing pins guava TreeRangeSet's NavigableMap<Cut<C>,Range<C>>
// navigation calls: the Object/Comparable-typed key argument must be wrapped in a `(Cut<C>)` cast.
// Disabling the fix via the kill-switch must reintroduce the "cannot be converted to Cut<C>" error.
func TestParameterizedArgCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := parameterizedArgCastErrCount(t, jarPath, false) // fix ON
	off := parameterizedArgCastErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("TreeRangeSet parameterized-arg-cast errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the parameterized-arg-cast error: ON=%d (want 0)", on)
	}
}
