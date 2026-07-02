package cross

// 承重测试:「返回裸型变的三元(条件)表达式缺 (T) 造型致 poly-typing 失败」治本 (CODEC_TODO 泛型擦除/返回造型族).
//
// 真实 guava ConfigurableValueGraph<N, V>.edgeValueOrDefault_internal:
//   protected final V edgeValueOrDefault_internal(N var1, N var2, V var3) {
//       GraphConnections var4 = (GraphConnections) this.nodeConnections.get(var1);
//       Object var5 = (var4 == null) ? null : var4.value(var2);   // var5 丢泛型退化成 Object
//       return (V) ((var5 == null) ? var3 : var5);                // 源码等价:整条三元受检造型成 V
//   }
// nodeConnections.get() 返回裸 raw GraphConnections, value() 遂只见 Object, var5 定型 Object。返回目标是
// 裸型变 V,而 `return cond ? var3(V) : var5(Object)` 是 poly 表达式:javac 按目标 V 逐臂定型,var5 那臂
// Object 无法当 V -> "incompatible types: bad type in conditional expression / Object cannot be converted
// to V"。三元自身 Type() 经 MergeTypes(V,Object) collapse 成 V,遮蔽了错配,故 typeVarReturnCast 的
// rawStr==retStr 早退不补造型。治本 (typeVarReturnCast 增三元分支:返回裸型变且某非 null 臂类型 != 该型变时,
// 整条三元补 (V) 造型 -> 三元变 standalone、按臂 LUB(=Object) 定型,再 unchecked (V) 造型,合法且保义)。
// JDEC_TERNARY_TYPEVAR_RET_CAST_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// ternaryTypeVarRetCastErrCount decompiles ConfigurableValueGraph under the kill-switch, recompiles it
// against the jar, and returns the count of the ternary-return-to-type-variable defect: the poly
// conditional `(var5 == null) ? var3 : var5` returned into `V` renders without the wrapping `(V)` cast,
// so javac rejects "bad type in conditional expression" (var5's Object arm cannot be typed as V). The
// substring is unique to this defect in the file (its other errors are unrelated NullableDecl import
// misses when the checkerframework dep is absent, which never carry this phrase).
func ternaryTypeVarRetCastErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_TERNARY_TYPEVAR_RET_CAST_OFF"
	const entry = "com/google/common/graph/ConfigurableValueGraph.class"
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
	dst := filepath.Join(dir, "ConfigurableValueGraph.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "ConfigurableValueGraph.java") {
			continue
		}
		// The poly ternary returned into V, its Object arm rejected without the wrapping (V) cast.
		if strings.Contains(line, "bad type in conditional expression") {
			n++
		}
	}
	return n
}

// TestTernaryTypeVarRetCastIsLoadBearing pins guava ConfigurableValueGraph.edgeValueOrDefault_internal:
// a conditional expression returned into a bare type variable V (`return (v == null) ? dflt : v;`, where
// `v` was read Object off a raw GraphConnections) must be wrapped in an unchecked `(V)` cast so the
// otherwise-poly ternary types standalone at its arm LUB. Disabling the fix via the kill-switch must
// reintroduce the "bad type in conditional expression" error.
func TestTernaryTypeVarRetCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := ternaryTypeVarRetCastErrCount(t, jarPath, false) // fix ON
	off := ternaryTypeVarRetCastErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("ConfigurableValueGraph ternary-return-to-typevar errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the ternary-return-to-typevar error: ON=%d (want 0)", on)
	}
}
