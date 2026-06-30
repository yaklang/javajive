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
	"os"
	"regexp"
	"testing"
)

// varargsSpreadRe matches the spread form `forArr(var0,var1)` (elements passed directly).
var varargsSpreadRe = regexp.MustCompile(`forArr\(var0\s*,\s*var1\)`)

// varargsArrayRe matches the un-spread array form `forArr(new Object[]{...})`.
var varargsArrayRe = regexp.MustCompile(`forArr\(new Object\[\]\{`)

func TestVarargsSpreadIsLoadBearing(t *testing.T) {
	seedBytes, err := os.ReadFile("testdata/regression/VarargsSpreadSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	helperBytes, err := os.ReadFile("testdata/regression/VarargsSpreadHelper.class")
	if err != nil {
		t.Fatalf("read helper seed: %v", err)
	}
	// Resolver feeds the varargs callee's bytes by binary internal name (default package -> bare name).
	resolver := func(internalName string) ([]byte, bool) {
		if internalName == "VarargsSpreadHelper" {
			return helperBytes, true
		}
		return nil, false
	}

	// Fix ON (default): the callee's `T...` component is recognized as a type variable, so the
	// javac-materialized `new Object[]{...}` is spread back to individual arguments.
	os.Unsetenv("JDEC_VARARGS_SPREAD_OFF")
	on, err := DecompileWithResolver(seedBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !varargsSpreadRe.MatchString(on) {
		t.Errorf("fix ON: expected spread `forArr(var0,var1)`, got:\n%s", on)
	}
	if varargsArrayRe.MatchString(on) {
		t.Errorf("fix ON: expected the explicit Object[] array to be gone, got:\n%s", on)
	}

	// Fix OFF: spreading disabled, so the faithful (but mis-inferring) explicit array form returns,
	// proving the spread pass -- not some unrelated rendering -- is what produced the spread form.
	t.Setenv("JDEC_VARARGS_SPREAD_OFF", "1")
	off, err := DecompileWithResolver(seedBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !varargsArrayRe.MatchString(off) {
		t.Errorf("fix OFF: expected the explicit `forArr(new Object[]{...})` array form to return, got:\n%s", off)
	}
}
