package javaclassparser

// 承重测试: 对**同类自有泛型方法**在 `this` 上的调用 (`this.has(objVal)`, 当前类声明 `boolean has(E)`),
// 必须从该方法自身的 Signature 还原形参类型, 给实参补回源码原有的 `(E)` 造型。被调方法的泛型签名位于
// 同类的另一个方法上, 描述符把形参擦除为 Object —— 这是泛型擦除阻断的最大剩余块
// (guava Forwarding*/集合家族 tailSet(E)/headSet(E)/... + fastjson2)。
// 安全边界: 只对**类作用域**类型变量造型, 绝不对方法作用域 `<T>`(调用点不在其作用域)或具体类型造型。
// kill-switch JDEC_GENERIC_SELFMETHOD_PARAM_OFF 关掉后造型消失, 证明承重。
// 种子 = 合成的 `SelfMethodArgSeed<E>`(声明 `boolean has(E)`, `check(Object)` 里 `this.has((E)o)`)。

import (
	"os"
	"regexp"
	"testing"
)

// genericSelfMethodArgCastRe matches the `(E)` cast re-synthesized on the argument to a same-class
// `this.has(...)` call, e.g. `this.has((E)(var1))`.
var genericSelfMethodArgCastRe = regexp.MustCompile(`this\.has\(\(E\)\(`)

func TestGenericSelfMethodArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GenericSelfMethodArgCast.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the Object argument is cast to the same-class method's E type variable.
	os.Unsetenv("JDEC_GENERIC_SELFMETHOD_PARAM_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !genericSelfMethodArgCastRe.MatchString(on) {
		t.Errorf("fix ON: expected a `(E)` cast on the same-class `this.has` arg, got:\n%s", on)
	}

	// Fix OFF (sub-switch): the cast disappears (legacy uncast arg), proving the same-class method
	// signature fallback is what re-synthesizes it rather than some unrelated pass.
	t.Setenv("JDEC_GENERIC_SELFMETHOD_PARAM_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if genericSelfMethodArgCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the same-class method arg cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
