package cross

// 承重测试: phase-1-post 遍历 rebindIncompatibleInvokeArgs 对 value-returning invokevirtual 的
// receiver / invokestatic 参数做两趟 load 重绑。
//
// 真实 fastjson2 ObjectReaderBaseModule:793:
// `var7.getParameters()` 其中 var7 解析为 Annotation[] (DFS 损坏: slot 7 的 null 初始化与
// Constructor 存储合流, 读绑到 null-init 的 Annotation[] 而非覆盖的 Constructor)。
// 字节码: offset 37 astore 7 (null) → offset 48 astore 7 (getDeclaredConstructor → Constructor)
// → offset 60 aload 7 → offset 62 invokevirtual getParameters()。
// rebindIncompatibleLoadForSink 无法治此: value-returning invokevirtual 在 phase-1 把 FCE 压栈,
// receiver 嵌在 FCE.Object 中, 永不到达 phase-2 的 putstatic/putfield sink。
//
// 治本 (rebindIncompatibleInvokeArgs): phase-1-post (opcodeIdToRef 已全填) 遍历所有 invoke 的 FCE,
// 对 receiver (非 static) 和 arguments (按声明参数类型) 检查 local-load SlotValue 的类型是否与
// callee 声明类/参数类型不兼容, 若不兼容则用 reachingStoresOf 找到类型匹配的到达 store 重绑。
// Kill-switch: JDEC_REBIND_INCOMPATIBLE_LOAD_OFF=1。

import (
	"os"
	"strings"
	"testing"
)

// rebindInvokeArgsErrCount decompiles the WHOLE fastjson2 jar under the kill-switch, then tree-compiles
// (deps-only cp) and counts "cannot find symbol.*getParameters" + "cannot be converted to (Class|MethodHandle)"
// errors across ObjectReaderBaseModule + JDKUtils (the invoke-arg slot-swap family).
func rebindInvokeArgsErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_REBIND_INCOMPATIBLE_LOAD_OFF"
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

	spec, ok := jarSpecs["fastjson2"]
	if !ok {
		t.Fatal("fastjson2 spec missing")
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	deps := resolveDeps(spec.depGlob)
	root := t.TempDir()
	files, _, _ := decompileAll(t, jarPath, root, 0)
	cp := withJfr(t, withSunMisc(t, strings.Join(deps, string(os.PathListSeparator))))
	outDir := t.TempDir()
	_, raw := treeCompileToDir(t, files, cp, outDir)
	count := 0
	for _, l := range strings.Split(raw, "\n") {
		// Count the invoke-arg slot-swap family: getParameters cannot find symbol (ObjectReaderBaseModule),
		// MethodHandle cannot be converted to Class / Throwable cannot be converted to MethodHandle (JDKUtils).
		if (strings.Contains(l, "ReaderAnnotationProcessor") || strings.Contains(l, "JDKUtils")) &&
			(strings.Contains(l, "cannot find symbol") || strings.Contains(l, "cannot be converted to")) {
			count++
		}
	}
	return count
}

// TestRebindInvokeArgsIsLoadBearing pins rebindIncompatibleInvokeArgs as load-bearing on fastjson2's
// ObjectReaderBaseModule:793 + JDKUtils:319: with the fix ON the invoke-arg slot-swap errors are gone
// from the tree compile; disabling via the kill-switch must reintroduce them.
func TestRebindInvokeArgsIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := rebindInvokeArgsErrCount(t, false)  // fix ON
	off := rebindInvokeArgsErrCount(t, true)  // fix OFF (kill-switch)
	t.Logf("fastjson2 tree invoke-arg slot-swap errors: ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("rebindIncompatibleInvokeArgs is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
