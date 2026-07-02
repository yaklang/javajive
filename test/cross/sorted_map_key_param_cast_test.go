package cross

// 承重测试:「SortedMap/NavigableMap<K,V> 的键形参导航方法(headMap/tailMap/subMap/floorKey...)缺 (K)
// 实参造型」治本 (CODEC_TODO 泛型擦除·JDK 方法形参造型族)。
//
// 真实 guava Maps$FilteredEntrySortedMap<K, V>.lastKey():
//   Object var2;
//   SortedMap var1 = this.sortedMap();          // sortedMap() 真返 SortedMap<K,V>
//   do { var2 = var1.lastKey();                  // var1 声明 raw 故 var2 = Object
//        if (this.apply(var2, (V)(this.unfiltered.get(var2)))) break;
//        else var1 = this.sortedMap().headMap((K)(var2));   // 源码带 (K) 造型
//   } while (true);
// headMap 的形参在字节码里被擦成 Object, 反编译不补造型; javac 却按 SortedMap<K,V>.headMap(K) 定型判
// "Object cannot be converted to K"。治本 (jdkMethodParamTypeArgIndex 增 sorted-map 键形参导航方法表,
// 使 instantiatedParamType 把 headMap 形参 0 解析为接收者 K, 既有 arg-cast 路径遂重下 (K) 造型)。
// JDEC_SORTED_MAP_KEY_PARAM_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// sortedMapKeyParamErrCount decompiles Maps$FilteredEntrySortedMap under the kill-switch, recompiles it
// against the jar, and counts the sorted-map key-parameter cast defect: the Object-typed key argument
// to `sortedMap().headMap(...)` passed without the `(K)` cast, so javac rejects "Object cannot be
// converted to K". The substring isolates precisely this defect (the file's other errors are
// flattened-`$` symbol lookups that never carry it).
func sortedMapKeyParamErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_SORTED_MAP_KEY_PARAM_OFF"
	const entry = "com/google/common/collect/Maps$FilteredEntrySortedMap.class"
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
	dst := filepath.Join(dir, "Maps$FilteredEntrySortedMap.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "Maps$FilteredEntrySortedMap.java") {
			continue
		}
		if strings.Contains(line, "Object cannot be converted to K") {
			n++
		}
	}
	return n
}

// TestSortedMapKeyParamCastIsLoadBearing pins guava Maps$FilteredEntrySortedMap.lastKey: an Object-typed
// key argument to `sortedMap().headMap(...)` (receiver recovered as SortedMap<K,V>) must be wrapped in a
// `(K)` cast, headMap's key parameter resolved to the receiver's K via the sorted-map key-parameter
// table. Disabling the fix via the kill-switch must reintroduce the "Object cannot be converted to K"
// error.
func TestSortedMapKeyParamCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := sortedMapKeyParamErrCount(t, jarPath, false) // fix ON
	off := sortedMapKeyParamErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("Maps$FilteredEntrySortedMap sorted-map key-param errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the sorted-map key-param error: ON=%d (want 0)", on)
	}
}
