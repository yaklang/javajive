package cross

// 承重测试: 三元表达式 `cond ? A : B` 赋值到具体类型局部 `T var` 时, 若某一 arm 的类型
// 与 T 不兼容 (sibling 类型, 如 List vs Map), javac 拒绝 "bad type in conditional expression"。
// JVM 层面两个 arm 都 astore 进同一槽位 (无 checkcast), 但 Java 源码层面需要显式 cast。
//
// 治本 (ternaryArmIncompatibleCast): 当且仅当恰好一个 arm 不可赋值到目标类型时, 给该 arm
// 包裹 `(T)` cast, 使条件表达式在目标类型处合流。Kill-switch: JDEC_TERNARY_ARM_CAST_OFF=1。
//
// 真实 fastjson2 JSONPathSegment$CycleNameSegment.eval:
// `List var8 = (var5_2 == 91) ? var1.readArray() : var1.readObject()`
// 其中 readArray():List, readObject():Map (sibling)。治本: `... : ((List)(var1.readObject()))`。

import (
	"os"
	"strings"
	"testing"
)

// ternaryArmCastErrCount decompiles the WHOLE fastjson2 jar under the kill-switch, then compiles
// ALL units in tree mode (deps-only classpath, matching the tree inventory metric), and counts
// "bad type in conditional expression" (the CycleNameSegment:172 signature). With the fix ON that
// error is gone; with the fix OFF (kill-switch) the Map arm of the ternary is not assignable to List.
func ternaryArmCastErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_TERNARY_ARM_CAST_OFF"
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
	// Tree-compile: deps-only classpath (NOT the original jar), matching TestJarTreeInventory. This
	// makes readObject()'s decompiled signature (Map) resolve from the decompiled sibling, not the
	// original jar — reproducing the "bad type in conditional expression" error when the cast is off.
	cp := withJfr(t, withSunMisc(t, strings.Join(deps, string(os.PathListSeparator))))
	outDir := t.TempDir()
	_, raw := treeCompileToDir(t, files, cp, outDir)
	count := 0
	for _, l := range strings.Split(raw, "\n") {
		if strings.Contains(l, "bad type in conditional expression") {
			count++
		}
	}
	return count
}

// TestTernaryArmCastIsLoadBearing pins ternaryArmIncompatibleCast as load-bearing on fastjson2's
// JSONPathSegment$CycleNameSegment.eval: with the fix ON the "bad type in conditional expression"
// signature is gone from the tree compile; disabling the cast via the kill-switch must reintroduce it.
func TestTernaryArmCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := ternaryArmCastErrCount(t, false) // fix ON
	off := ternaryArmCastErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 tree 'bad type in conditional expression' (CycleNameSegment): ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("ternaryArmIncompatibleCast is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
