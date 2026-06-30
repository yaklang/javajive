package cross

// Phase 3 承重测试 (Bug AH: 有限泛型实例化 / 类型变量沿数据流传播)。
//
// 它从真实 guava jar 取 PairwiseEquivalence (GA Group A 残留种子之一), 用生产路径反编译后整组重编译,
// 断言「修复 ON 的 javac 错误数 严格少于 修复 OFF」——即 JDEC_GENERIC_INFER_OFF 关掉治本后必能复现缺陷
// (load-bearing)。根因: `Iterator var = iterable.iterator()` 的返回类型按 descriptor 擦除成 raw Iterator,
// `var.next()` 遂返回 Object, 传入 `Equivalence<? super T>.equivalent(? super T,...)` → "Object cannot be
// converted to CAP#1"。治本把已知 JDK 容器方法 (Iterable.iterator()->Iterator<E>、Iterator.next()->E) 按
// 接收者实参实例化返回类型, 使 var 定型 Iterator<T>、var.next() 定型 T。

import "testing"

// TestGenericInstantiationIsLoadBearing pins guava PairwiseEquivalence: instantiating the JDK
// iterator()/next() return types from the receiver's type arguments must strictly reduce the unit's
// recompile errors, and disabling it via JDEC_GENERIC_INFER_OFF must reproduce the defect.
func TestGenericInstantiationIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}
	const prefix = "com/google/common/base/PairwiseEquivalence"

	on := groupRecompileErrorsSwitch(t, jarPath, prefix, "JDEC_GENERIC_INFER_OFF", false) // fix ON
	off := groupRecompileErrorsSwitch(t, jarPath, prefix, "JDEC_GENERIC_INFER_OFF", true) // fix OFF
	t.Logf("PairwiseEquivalence recompile errors: ON=%d OFF=%d", on, off)

	if off <= on {
		t.Errorf("generic-instantiation fix is NOT load-bearing: ON=%d OFF=%d (OFF must reproduce more errors)",
			on, off)
	}
}
