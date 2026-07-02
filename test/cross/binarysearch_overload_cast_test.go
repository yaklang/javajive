package cross

// 承重测试:「兄弟类方法签名按 (name, arity) 收键, 遇同 arity 重载即丢弃(歧义), 致 calleeParamIsErasedTypeVar
// 拿不到正确重载的泛型 Signature、看不出该形参是擦除型变, 遂对 K 形参补上有害的 `(Comparable)` 造型」治本
// (CODEC_TODO 泛型擦除·兄弟签名 descriptor 键重载消歧)。
//
// 真实 guava:`SortedLists` 有两个 5 参 binarySearch 重载——
//   <E extends Comparable> binarySearch(List<? extends E>, E, KPB, KAB)                     (擦除 (List,Comparable,..))
//   <E,K extends Comparable> binarySearch(List<E>, Function<? super E,K>, K, KPB, KAB)      (擦除 (List,Function,Comparable,..))
// 二者 arity=5 键碰撞被丢, 于是 `ImmutableRangeMap`/`ImmutableRangeSet` 里
//   SortedLists.binarySearch((List)ranges, Range.lowerBoundFn(), Cut.belowValue(key), ..)
// 的第 3 实参(真类型 K=Cut<C>)被误补 `(Comparable)` 造型, 令 javac 对 K 同时得
// `equality: Cut<C>`(来自 Function 见证)与 `lower bound: Comparable`(来自造型)两冲突约束,
// 报 `no suitable method found for binarySearch(...)`(共 7 处: ImmutableRangeMap 5 + ImmutableRangeSet 2)。
// 治法: 兄弟签名图额外按 descriptor 收键(每重载唯一, 不受 arity 碰撞丢弃影响), calleeParamIsErasedTypeVar
// 优先按调用点 descriptor 精确取重载 Signature, 遂识出第 3 形参是型变 K、抑制该 `(Comparable)` 造型。
// JDEC_SIBLING_DESC_SIG_OFF=1 关掉 descriptor 键消歧必复现。
//
// 该缺陷仅在整树(全部兄弟类一起 javac)下暴露(单文件对原始 jar 编译时 jar 内正确签名会"治愈"推断),
// 故本测试走整 jar 反编译 + 整树重编译口径。

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// binarySearchOverloadErrCount decompiles the WHOLE guava jar under the kill-switch, recompiles the
// whole tree, and counts the binarySearch overload-resolution defect. The substring isolates precisely
// this defect.
func binarySearchOverloadErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_SIBLING_DESC_SIG_OFF"
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

	spec := jarSpecs["guava"]
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}
	javac := lookJavac(t)
	deps := resolveDeps(spec.depGlob)
	cp := withSunMisc(t, strings.Join(deps, string(os.PathListSeparator)))

	root := t.TempDir()
	files, _, _ := decompileAll(t, jarPath, root, 0)
	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000")
	if cp != "" {
		args = append(args, "-cp", cp)
	}
	args = append(args, "-d", outDir)
	args = append(args, files...)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ": error:") && strings.Contains(line, "no suitable method found for binarySearch") {
			n++
		}
	}
	return n
}

// TestBinarySearchOverloadResolveIsLoadBearing pins guava's SortedLists.binarySearch call sites: the
// K-typed key argument must NOT be wrapped in a `(Comparable)` cast (which would pin K to Comparable and
// clash with the Function witness's K=Cut<C>). Disabling the descriptor-keyed overload disambiguation
// via the kill-switch must reintroduce the "no suitable method found for binarySearch" errors.
func TestBinarySearchOverloadResolveIsLoadBearing(t *testing.T) {
	lookJavac(t)
	if resolveJar(jarSpecs["guava"].relPath) == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := binarySearchOverloadErrCount(t, false) // fix ON
	off := binarySearchOverloadErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("guava binarySearch overload-resolution errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the binarySearch overload-resolution error: ON=%d (want 0)", on)
	}
}
