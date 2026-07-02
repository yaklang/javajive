package javaclassparser

// 承重测试: JDEC_PARAM_FIELD_RAW_CAST_OFF 的 this 值扩展。泛型类 X<K,V> 有字段 X<V,K> inverse(型实参
// 交换), 无参构造器 `this.inverse = (X) this`。this 的值类型是原始 X(无实参), 但 javac 按类自身参数化
// X<K,V> 定型, 与字段 X<V,K> 同 raw 异参不可转; 源码 raw (X) 造型被字节码擦除(no-op 无 checkcast),
// 反编译得裸 `this.inverse = this`。治法: 重建 this 的自身参数化 X<K,V> 后按同擦除异参补 raw (X) 造型。
// 镜像 guava RegularImmutableBiMap `this.inverse = this`(字段声明 RegularImmutableBiMap<V,K>)。
// 单类即复现(字段签名在本类、this 为同类), 无需 resolver。raw 造型对同擦除永远合法, 只会修好绝不新增错误。

import (
	"os"
	"strings"
	"testing"
)

func TestFieldThisRawCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/FieldThisRawCastSeed.class")
	if err != nil {
		t.Fatalf("read FieldThisRawCastSeed seed: %v", err)
	}

	os.Unsetenv("JDEC_PARAM_FIELD_RAW_CAST_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (rawcast ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.inverse = (FieldThisRawCastSeed) (this)") {
		t.Errorf("rawcast ON: expected `this.inverse = (FieldThisRawCastSeed) (this)`, got:\n%s", on)
	}

	t.Setenv("JDEC_PARAM_FIELD_RAW_CAST_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (rawcast OFF) failed: %v", err)
	}
	if strings.Contains(off, "(FieldThisRawCastSeed) (this)") {
		t.Errorf("rawcast OFF: expected the raw cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
