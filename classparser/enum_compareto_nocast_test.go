package javaclassparser

// 承重测试: 调用 java.lang.Enum<E>.compareTo(E) 时不再给实参补多余的 `(Enum)` 上行造型
// (jdkCalleeParamIsErasedTypeVar, kill-switch JDEC_ENUM_COMPARETO_NOCAST_OFF)。
//
// 镜像 guava AbstractService/ServiceManager `state().compareTo(State.RUNNING)`: compareTo 的描述符把
// 类型变量 E 擦除为其 bound `java.lang.Enum`, 实参造型逻辑遂把具体枚举常量上行造型成裸 `Enum`
// (`compareTo((Enum) State.RUNNING)`), 破坏 compareTo(E) 真实签名 → javac
// "Enum cannot be converted to <ConcreteEnum>"。实参本就是流入该形参的同一具体枚举(必可赋), 故丢弃
// 该 no-op 上行造型行为保持, 并让 javac 推断出 E。kill-switch 置位后恢复造型, 证明承重。

import (
	"os"
	"strings"
	"testing"
)

func TestEnumCompareToNoCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/EnumCompareToSeed.class")
	if err != nil {
		t.Fatalf("read EnumCompareToSeed seed: %v", err)
	}

	// Fix ON (default): the enum argument is rendered plainly, no `(Enum)` upcast.
	os.Unsetenv("JDEC_ENUM_COMPARETO_NOCAST_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "compareTo(EnumCompareToSeed$State.RUNNING)") {
		t.Errorf("fix ON: expected plain `compareTo(EnumCompareToSeed$State.RUNNING)`, got:\n%s", on)
	}
	if strings.Contains(on, "(Enum)(EnumCompareToSeed$State.RUNNING)") {
		t.Errorf("fix ON: must NOT upcast the enum arg to `(Enum)`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the spurious `(Enum)` upcast returns -- the exact "Enum cannot be converted
	// to <ConcreteEnum>" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_ENUM_COMPARETO_NOCAST_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "(Enum)(EnumCompareToSeed$State.RUNNING)") {
		t.Errorf("fix OFF: expected the `(Enum)` upcast fallback, got:\n%s", off)
	}
}
