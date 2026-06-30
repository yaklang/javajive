package javaclassparser

// 承重测试: 对**私有同类自有泛型方法**的调用 (`this.sink(k, objVal)`, 当前类声明 `private void sink(K,V)`)。
// 私有方法在 Java 8 字节码里走 invokespecial(与 `super.m()` 同一指令), 旧逻辑一刀切跳过所有 invokespecial,
// 导致私有同类泛型方法的实参 `(V)` 造型丢失(guava AbstractBiMap `this.updateInverseMap(k,b,objVal,v)`)。
// 修复: invokespecial 仍要区分目标类 —— 目标类==当前类即私有同类调用, 其签名在 funcCtx.MethodSignatures 中,
// 必须照常补 `(V)` 造型; 目标类!=当前类才是 super 调用, 跳过。
// 安全边界: 只对**类作用域**类型变量造型; super 调用绝不误判为同类。
// 聚焦 kill-switch JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF 关掉后恢复旧的 invokespecial 一刀切, 造型消失, 证明承重。
// 种子 = 合成的 `PrivateSelfMethodSeed<K,V>`(--release 8 编译, 私有 `sink(K,V)`, `put()` 里 `this.sink(k,(V)raw)`)。

import (
	"os"
	"regexp"
	"testing"
)

// privateSelfMethodArgCastRe matches the `(V)` cast re-synthesized on the argument to a private
// same-class `this.sink(...)` invokespecial call, e.g. `this.sink(var1,(V)(var2))`.
var privateSelfMethodArgCastRe = regexp.MustCompile(`this\.sink\(var1,\(V\)\(`)

func TestPrivateSelfMethodArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/PrivateSelfMethodArgCast.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the Object argument to the PRIVATE same-class generic method is cast to V.
	os.Unsetenv("JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF")
	os.Unsetenv("JDEC_GENERIC_SELFMETHOD_PARAM_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !privateSelfMethodArgCastRe.MatchString(on) {
		t.Errorf("fix ON: expected a `(V)` cast on the private same-class `this.sink` arg, got:\n%s", on)
	}

	// Sub-switch OFF: restore the legacy blanket-skip of invokespecial -> the private same-class cast
	// disappears, proving this extension (not the public-method path) is what re-synthesizes it.
	t.Setenv("JDEC_GENERIC_SELFMETHOD_PRIVATE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (sub-switch OFF) failed: %v", err)
	}
	if privateSelfMethodArgCastRe.MatchString(off) {
		t.Errorf("sub-switch OFF: expected the private same-class arg cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
