package cross

// 承重测试:「boolean 标志(存入 Z 字段)复用了不相交活跃区间的 int 循环计数器槽」治本 (CODEC_TODO disjoint 槽族).
//
// 真实 fastjson2 JSONPathTypedMultiIndexes.<init>:
//   int[] indexes = new int[paths.length];
//   for (int i = 0; i < indexPaths.length; i++) indexes[i] = ...;   // slot 11 = i (int)
//   this.indexes = indexes;
//   boolean duplicate = false;                                      // slot 11 复用 (iconst_0; istore 11)
//   ...; if (...) { duplicate = true; break; }
//   this.duplicate = duplicate;                                     // putfield duplicate:Z
//
// `duplicate = false` 被编成 `iconst_0; istore 11`, 是 int-0/1 字面量; AssignVarGuarded 视其与槽内
// (已死的) int 循环计数器 ref 同类而续用, 把两个不相交活跃区间并成一个变量; 槽类型随后从 `putfield
// duplicate:Z` / `!duplicate` 解析为 boolean, 于是先前的 int 循环用法 (`i < len` / `array[i]` / `i++`)
// 报 "bad operand types '<'" / "boolean cannot be converted to int" x2 / "bad operand type boolean
// for unary operator '++'" (本方法 4 行)。治本 (reachingBoolFieldSlotSplit): 该 store 的值前向流入
// boolean 字段 (putfield/putstatic Z) 且与 current int 槽不同 web 时, 把 0/1 转成 boolean, 让
// AssignVarGuarded 新铸一个 boolean 变量, 把 boolean 标志区间从 int 计数器区间拆开。
// JDEC_BOOL_FIELD_SLOT_SPLIT_OFF=1 关掉治本必复现。

import "testing"

// TestBoolFieldSlotSplitIsLoadBearing pins fastjson2 JSONPathTypedMultiIndexes.<init>: a boolean flag
// stored into a Z field, reusing a JVM slot that a disjoint earlier range used as an int loop counter,
// must split into two variables (int counter + boolean flag) rather than merge into one. Disabling the
// fix via the kill-switch must reintroduce the int/boolean "incompatible types" / "bad operand" errors.
func TestBoolFieldSlotSplitIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const sw = "JDEC_BOOL_FIELD_SLOT_SPLIT_OFF"
	const entry = "com/alibaba/fastjson2/JSONPathTypedMultiIndexes.class"
	const fileSubstr = "JSONPathTypedMultiIndexes.java"
	substrs := []string{
		"boolean cannot be converted to int",
		"bad operand type boolean for unary operator",
		"bad operand types for binary operator '<'",
	}
	entries := []string{entry}

	on := classConvErrCount(t, sw, jarPath, entries, fileSubstr, substrs, false) // fix ON
	off := classConvErrCount(t, sw, jarPath, entries, fileSubstr, substrs, true) // fix OFF (kill-switch)
	t.Logf("JSONPathTypedMultiIndexes int/boolean slot errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all int/boolean slot errors: ON=%d (want 0)", on)
	}
}
