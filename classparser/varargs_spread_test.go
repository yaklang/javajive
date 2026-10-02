package javaclassparser

// 承重测试: 泛型可变参数方法 (component 为类型变量, `<T> Iterator<T> forArr(T... xs)`) 的调用点,
// 字节码把 `forArr(a, b)` 物化成 `forArr(new Object[]{a, b})`; 忠实保留该显式 Object[] 会把 T 钉成
// Object, 与调用方 `Iterator<N>` 的返回推断冲突 (javac: "inference variable T has incompatible
// bounds: Object, N", 即 guava EndpointPair.iterator()/ImmutableMultiset.of() 家族)。治法把数组字面量
// 重新展开成 `forArr(a, b)`, 让 javac 从实参类型推断 T。
//
// 关键: 该治本依赖跨类 resolver 读被调方法的泛型 Signature (判定 component 是类型变量), 故必须用
// DecompileWithResolver(两类: 调用方 VarargsSpreadSeed + 被调方 VarargsSpreadHelper)。kill-switch
// JDEC_VARARGS_SPREAD_OFF 关掉后回退到显式 Object[] 数组形, 证明承重。

import (
	"regexp"
	"testing"
)

// varargsSpreadRe matches the spread form `forArr(var0,var1)` (elements passed directly).
var varargsSpreadRe = regexp.MustCompile(`forArr\(var0\s*,\s*var1\)`)

// varargsArrayRe matches the un-spread array form `forArr(new Object[]{...})`.
var varargsArrayRe = regexp.MustCompile(`forArr\(new Object\[\]\{`)

func TestVarargsSpreadIsLoadBearing(t *testing.T) {
	raw := reviewedRemainingSAMRaw(t, "VarargsSpreadSeed")
	helper := reviewedRemainingSAMRaw(t, "VarargsSpreadHelper")
	assertReviewedTypeVarMethod(t, raw, "make", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/util/Iterator;", "<N:Ljava/lang/Object;>(TN;TN;)Ljava/util/Iterator<TN;>;")
	assertReviewedTypeVarMethod(t, helper, "forArr", "([Ljava/lang/Object;)Ljava/util/Iterator;", "<T:Ljava/lang/Object;>([TT;)Ljava/util/Iterator<TT;>;")
	assertReviewedTypeVarInvoke(t, "testdata/regression/VarargsSpreadSeed.class", "make", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/util/Iterator;", 12, 184, "VarargsSpreadHelper", "forArr", "([Ljava/lang/Object;)Ljava/util/Iterator;")
	reviewedSeedSources(t, "testdata/regression/VarargsSpreadSeed.class", "JDEC_VARARGS_SPREAD_OFF", true, func(source string) {
		body := reviewedSourceMethod(t, source, `Iterator<N>\s+make\(`)
		params := requireReviewedPattern(t, body, `make\(N\s+(\w+),\s*N\s+(\w+)\)`)
		carrier := requireReviewedPattern(t, body, `Object\[\]\s+(\w+)\s*=\s*new Object\[\]\{`+params[1]+`,`+params[2]+`\};`)[1]
		requireReviewedPattern(t, body, `return\s+\(Iterator<N>\)\s*\(Iterator\)\s*\(VarargsSpreadHelper\.forArr\(`+carrier+`\)\);`)
	})
}
