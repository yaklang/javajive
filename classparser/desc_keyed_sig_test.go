package javaclassparser

// 承重测试: 同类方法按 (name, arity) 键遇同实参数重载(普通 `add(E)` 与变长 `add(E...)`)会被当作二义
// 而整体丢弃, 致 `this.add(objVal)` 漏补被擦除的 `(E)` 造型。descriptor 唯一, 据此精确解析可救回。
//
// 镜像 guava `ImmutableCollection$Builder.add(E)` vs `add(E...)`、`ImmutableMultimap$Builder.putAll(K,
// Iterable)` vs `putAll(K, V...)`: 两重载同实参数, 老的 arity 键表把二者一并删除, `sameClassMethodParamType`
// 拿不到签名→不补造型, javac 报 `Object cannot be converted to E/K`。治法(kill-switch
// JDEC_SAMECLASS_DESC_SIG_OFF): 增补 (name, 精确 descriptor) 键表(JVM 内 name+descriptor 唯一, 永不二义),
// 调用侧用 `FunctionCallExpression.Descriptor` 精确命中重载→恢复形参型变→补 `(E)`。种子: `add(E)` +
// `add(E...)` 同类共存 + `this.add((E) o)`(编译擦除后 `(E)` 消失)。ON=`(E)(var1)` / OFF=裸 `var1`。

import (
	"os"
	"strings"
	"testing"
)

func TestDescKeyedSameClassSigIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/DescKeyedSigSeed.class")
	if err != nil {
		t.Fatalf("read DescKeyedSigSeed seed: %v", err)
	}

	// Fix ON (default): the descriptor-keyed table disambiguates add(E)/add(E...) and re-emits `(E)`.
	os.Unsetenv("JDEC_SAMECLASS_DESC_SIG_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.add((E)(var1))") {
		t.Errorf("fix ON: expected descriptor-keyed cast `this.add((E)(var1))`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the descriptor-keyed table is not populated, so the arity path abandons the
	// ambiguous overload and the bare `this.add(var1)` returns -- the exact "Object cannot be converted to
	// E" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_SAMECLASS_DESC_SIG_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "this.add((E)(var1))") {
		t.Errorf("fix OFF: expected the `(E)` cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
