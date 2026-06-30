package javaclassparser

// 承重测试: 方法引用实例化类型上行 (kill-switch JDEC_METHODREF_INSTANTIATED_TYPE_OFF)
//
// 方法引用本身没有目标类型, 字节码里 invokedynamic 的 instantiatedMethodType(第 3 个 bootstrap 实参)
// 记录了实例化签名 `(List)List`。修复前反编译器把承载方法引用的局部类型成 RAW `Function`(SAM 为
// apply(Object)), 重建出的 `Function f = Collections::synchronizedList` 被 javac 拒收
// ("incompatible types: invalid method reference")。上行为 `Function<List, List>` 后, 局部声明带泛型,
// 方法引用得以绑定。镜像 fastjson2 ObjectReaderImpl{List,ListStr,Map,MapMultiValueType}
// `var = Collections::synchronized*/unmodifiable*` (整 jar tree 229→... 中该桶 26→1, A/B -23)。

import (
	"os"
	"strings"
	"testing"
)

func methodRefSeedDecompile(t *testing.T) string {
	t.Helper()
	seed, err := os.ReadFile("testdata/regression/MethodRefInstantiatedSeed.class")
	if err != nil {
		t.Fatalf("read MethodRefInstantiatedSeed seed: %v", err)
	}
	out, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	return out
}

func TestMethodRefInstantiatedTypeIsLoadBearing(t *testing.T) {
	// Fix ON (default): the method-reference local carries the instantiated `Function<List, List>`,
	// so `Function<List, List> f = Collections::synchronizedList` recompiles.
	os.Unsetenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF")
	on := methodRefSeedDecompile(t)
	if !strings.Contains(on, "Function<List, List>") {
		t.Errorf("fix ON: expected parameterized local `Function<List, List>`, got:\n%s", on)
	}
	if !strings.Contains(on, "Collections::synchronizedList") {
		t.Errorf("fix ON: expected the method reference `Collections::synchronizedList`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the local degrades to RAW `Function` -- the exact "invalid method
	// reference" recompile blocker the upgrade removes -- proving it is load-bearing.
	t.Setenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF", "1")
	off := methodRefSeedDecompile(t)
	if strings.Contains(off, "Function<List, List>") {
		t.Errorf("fix OFF: expected RAW `Function` (no parameterization), got:\n%s", off)
	}
	if !strings.Contains(off, "Collections::synchronizedList") {
		t.Errorf("fix OFF: expected the method reference `Collections::synchronizedList`, got:\n%s", off)
	}
}
