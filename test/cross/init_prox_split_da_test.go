package cross

// 承重测试: initProximateSplitSlotDecl 在渲染层为 bare `Type varN;` 声明追加默认初始化器
// (`= null` / `= 0`), 当方法体中存在近邻 Object dead-store sibling (proximity gate ≤10 行)
// 或 varN 被多次赋值 (multi-assign gate ≥2 次) 时。修 slot-split definite-assignment 错误。
//
// 真实 fastjson2 JSON.copyTo: slot 16 跨分支 String(DateUtils.format)/Object(var15/copy),
// 反编译器拆成 var16(String)+var17(Object dead-store), else 分支未赋值 var16 → javac 拒。
// 治本: `String var16;` → `String var16 = null;`。
// Kill-switch: JDEC_INIT_PROX_SPLIT_OFF=1。

import (
	"os"
	"strings"
	"testing"
)

// initProxSplitDAErrCount decompiles the WHOLE fastjson2 jar under the kill-switch, then tree-compiles
// and counts "might not have been initialized" errors for JSON.java's var16 (the #JSON-4367 signature).
func initProxSplitDAErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_INIT_PROX_SPLIT_OFF"
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
		// Count JSON.java var16 "might not have been initialized" specifically.
		if strings.Contains(l, "JSON.java") && strings.Contains(l, "var16") && strings.Contains(l, "might not have been initialized") {
			count++
		}
	}
	return count
}

// TestInitProxSplitDAIsLoadBearing pins initProximateSplitSlotDecl as load-bearing on fastjson2's
// JSON.copyTo: with the fix ON the JSON.java:4367 "var16 might not have been initialized" is gone
// from the tree compile; disabling via the kill-switch must reintroduce it.
func TestInitProxSplitDAIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := initProxSplitDAErrCount(t, false) // fix ON
	off := initProxSplitDAErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 tree JSON.java 'var16 might not have been initialized': ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("initProximateSplitSlotDecl is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
