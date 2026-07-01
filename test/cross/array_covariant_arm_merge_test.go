package cross

// 承重测试:「两个不相交臂把协变数组(new String[] / new Object[])存入同一槽」治本 (CODEC_TODO disjoint 槽族).
//
// 真实 fastjson2 CSVReaderUTF8/UTF16.readLineValues (LVT 确证 slot 2 全程是单一 `Object[] values`):
//   Object[] values = null;                         // null-init; slot 2
//   if (strings) values = new String[size];         // 臂: String[] (子类数组)
//   else         values = new Object[size];         // 臂: Object[] (父类数组)
//   ...
//   if (values != null) { if (i < values.length) values[i] = value; }   // 合流后使用
//   return values;
//
// DFS 顺序里 null-init 先采纳 String[](null-adopt-once),槽 ref 被定型 String[];Object[] 臂随后存入
// current=该 ref。AssignVarGuarded 没有数组协变规则(int 类目/裸泛型 merge 均不适用、null-adopt 已耗尽),
// 于是新铸一个 Object[] 变量;合流后的 `values[i] = value` 绑到 String[] ref,报
// "Object cannot be converted to String"(UTF8 两行 + UTF16 一行 = 3 行)。治本
// (reachingRefSlotArrayCovariantArmMerge):两臂都是等维引用数组、元素有可证父类型(此处 Object)、且 phi 证明
// 收敛为一个变量时,把 current 拓宽到元素 LUB 数组(Object[])并续用,两臂都变成对同一 Object[] 变量的普通再赋值。
// JDEC_REF_SLOT_ARRAY_COVARIANT_ARM_MERGE_OFF=1 关掉治本必复现。

import "testing"

// TestArrayCovariantArmMergeIsLoadBearing pins fastjson2 CSVReaderUTF8/UTF16.readLineValues: two
// disjoint arms store covariantly-related reference arrays (`new String[]` / `new Object[]`) into one
// slot the LVT confirms is a single `Object[] values`. The decompiler must widen the shared ref to the
// element-LUB array (Object[]) so both arms become plain reassignments, not split a String[] variable
// off that breaks the post-merge `values[i] = value` ("Object cannot be converted to String").
// Disabling the fix via the kill-switch must reintroduce those assignment errors.
func TestArrayCovariantArmMergeIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const sw = "JDEC_REF_SLOT_ARRAY_COVARIANT_ARM_MERGE_OFF"
	entries := []string{
		"com/alibaba/fastjson2/support/csv/CSVReaderUTF8.class",
		"com/alibaba/fastjson2/support/csv/CSVReaderUTF16.class",
	}
	substrs := []string{"Object cannot be converted to String"}

	// fileSubstr "" so both CSVReader units are counted; the substring already isolates the
	// covariant-array assignment error (the residual `Object[] cannot be converted to T` is a distinct
	// generic-return defect that does not match this substring).
	on := classConvErrCount(t, sw, jarPath, entries, "", substrs, false)  // fix ON
	off := classConvErrCount(t, sw, jarPath, entries, "", substrs, true)  // fix OFF (kill-switch)
	t.Logf("CSVReaderUTF8/UTF16 covariant-array slot errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all covariant-array slot errors: ON=%d (want 0)", on)
	}
}
