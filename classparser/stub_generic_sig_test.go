package javaclassparser

// 承重测试: 无法反编译而降级为 stub 的方法, 其签名应优先取泛型 Signature 属性而非擦除后的 descriptor,
// 否则丢失 `<...>` 会破坏该方法在其调用点的重载择优。
//
// 镜像 guava Joiner: `appendTo(StringBuilder, Iterator<?>)` 方法体反编译 panic 降级为 stub, 老实现按
// descriptor 渲染成 raw `appendTo(StringBuilder, Iterator)`, 使调用点 `this.appendTo(sb, it)` 在它与
// 泛型 `appendTo(A extends Appendable, Iterator<?>)` 之间无法择优 → `reference to appendTo is ambiguous`。
// 治法(kill-switch JDEC_STUB_GENERIC_SIG_OFF): stub 在方法无自身形参型变(签名以 '(' 开头)且签名形参数与
// descriptor 一致时, 用泛型 Signature 的形参/返回类型渲染。种子用 native 方法(无 Code, 必走 stub 路径)
// 携带 `Iterator<?>` 形参确定复现。ON=`Iterator<?>` / OFF=裸 `Iterator`。

import (
	"os"
	"strings"
	"testing"
)

func TestStubGenericSigIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/StubGenericSigSeed.class")
	if err != nil {
		t.Fatalf("read StubGenericSigSeed seed: %v", err)
	}

	// Fix ON (default): the stub renders the generic Signature parameter type `Iterator<?>`.
	os.Unsetenv("JDEC_STUB_GENERIC_SIG_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "consume(StringBuilder var0, Iterator<?> var1)") {
		t.Errorf("fix ON: expected generic stub param `Iterator<?>`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the stub falls back to the erased descriptor `Iterator` -- the raw form that
	// breaks overload specificity at call sites -- proving the generic-signature rendering is load-bearing.
	t.Setenv("JDEC_STUB_GENERIC_SIG_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "consume(StringBuilder var0, Iterator var1)") || strings.Contains(off, "Iterator<?>") {
		t.Errorf("fix OFF: expected the erased `Iterator` param (no `<?>`), got:\n%s", off)
	}
}
