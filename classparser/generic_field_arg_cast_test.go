package javaclassparser

// 承重测试: 对**同类字段接收者**上的 JDK 泛型方法调用 (`this.fn.accept(t, x)`, fn 是字段
// `BiConsumer<T,V>`), 必须从字段 Signature 还原接收者类型参数, 给实参补回源码原有的 `(V)` 造型。
// 字段访问值只带擦除描述符 (raw `BiConsumer`), 故需 ClassContext.FieldSignatures 旁路 ——
// 这是泛型擦除阻断的最大剩余块 (fastjson2 FieldReaderBigDecimalFunc 家族 + guava)。
// kill-switch JDEC_GENERIC_PARAM_FIELD_OFF 关掉字段旁路后造型消失, 证明承重。
// 种子 = 合成的 `GenFieldArgSeed<T,V>` (字段 `BiConsumer<T,V> fn`, 调 `fn.accept(t,(V)d)`)。

import (
	"os"
	"regexp"
	"testing"
)

// genericFieldArgCastRe matches the `(V)` cast re-synthesized on the argument to a field-receiver
// BiConsumer.accept call, e.g. `.accept(var1,(V)(new BigDecimal(var2)))`.
var genericFieldArgCastRe = regexp.MustCompile(`\.accept\(var\d+,\(V\)\(`)

func TestGenericFieldArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GenericFieldArgCast.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the argument is cast to the field receiver's V type arg.
	os.Unsetenv("JDEC_GENERIC_PARAM_FIELD_OFF")
	os.Unsetenv("JDEC_GENERIC_PARAM_INFER_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !genericFieldArgCastRe.MatchString(on) {
		t.Errorf("fix ON: expected a `(V)` cast on the field-receiver accept arg, got:\n%s", on)
	}

	// Fix OFF (field sub-switch): the cast disappears (legacy uncast arg), proving the field-signature
	// fallback is what re-synthesizes it rather than some unrelated pass.
	t.Setenv("JDEC_GENERIC_PARAM_FIELD_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if genericFieldArgCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the field-receiver arg cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
