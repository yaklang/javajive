package cross

// 承重测试: rebindIncompatibleLoadForSink 的数组类型放宽 + var-user 注册。
//
// 真实 fastjson2 TypeUtils.<clinit>:
// slot 0 的 null 初始化 (offset 825) 被 DFS 损坏解析为 Class[] (同槽后到的数组复用),
// 但真实到达 store 是 offset 841 getDeclaredField → Field。putstatic FIELD_JSON_OBJECT_1x_map
// (Field 字段) 消费 slot 0 的 aload (Class[]) → "Class[] cannot be converted to Field"。
//
// 治本 1 (数组放宽): rebindIncompatibleLoadForSink 原先对数组类型值 bail (ClassFQNOf 对数组返 false)。
// 修法: 当值类型是数组而 sink 类型不是数组 (或反之), 判定为真不兼容对, 继续到 reaching-store 查找。
//
// 治本 2 (var-user 注册): rebind 到正确 ref 后, 在 varUserMap 注册该 ref 的新 user (sink opcode),
// 使 var-fold 的用户计数反映该 phase-2 读。否则被 rebind 的 ref 看起来 single-use (phase-1 计数
// 不含该 sink), 会被 single-use-fold 折掉声明, sink 读到未声明变量。
// Kill-switch: JDEC_REBIND_INCOMPATIBLE_LOAD_OFF=1。

import (
	"os"
	"strings"
	"testing"
)

// rebindArraySinkErrCount decompiles the WHOLE fastjson2 jar under the kill-switch, then tree-compiles
// (deps-only cp) and counts "Class[] cannot be converted to Field" errors in TypeUtils.java.
func rebindArraySinkErrCount(t *testing.T, killOff bool) int {
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
		if strings.Contains(l, "TypeUtils.java") && strings.Contains(l, "cannot be converted to Field") {
			count++
		}
	}
	return count
}

// TestRebindArraySinkIsLoadBearing pins the array-type relaxation + var-user registration in
// rebindIncompatibleLoadForSink as load-bearing on fastjson2's TypeUtils.<clinit>: with the fix ON
// the "Class[] cannot be converted to Field" error is gone from the tree compile; disabling via
// the kill-switch must reintroduce it.
func TestRebindArraySinkIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := rebindArraySinkErrCount(t, false) // fix ON
	off := rebindArraySinkErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 tree TypeUtils 'cannot be converted to Field': ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("rebindIncompatibleLoadForSink array relaxation is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
