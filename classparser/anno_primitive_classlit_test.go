package javaclassparser

// 承重测试: 注解默认值里的 PRIMITIVE 类字面量 (void.class / int.class / boolean.class) 必须渲染成
// 原始关键字, 不能过 ShortTypeName -> SafeIdentifier (它给每个 Java 关键字追加 '_', 把 `void.class`
// 变成不可编译的 `void_.class`)。引用类型 (String.class) 不受影响。kill-switch
// JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF 置位则回退到旧的损坏形态, 证明造型承重于本修复。
// 真实命中: fastjson2 @JSONType `builder() default void.class`。

import (
	"os"
	"strings"
	"testing"
)

func TestAnnoPrimitiveClassLitIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AnnoPrimClassLitSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): primitive class literals render as raw keywords.
	os.Unsetenv("JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	for _, want := range []string{"void.class", "int.class", "boolean.class", "String.class"} {
		if !strings.Contains(on, want) {
			t.Errorf("fix ON: expected %q in output, got:\n%s", want, on)
		}
	}
	for _, bad := range []string{"void_.class", "int_.class", "boolean_.class"} {
		if strings.Contains(on, bad) {
			t.Errorf("fix ON: must not emit SafeIdentifier-mangled %q, got:\n%s", bad, on)
		}
	}

	// Fix OFF: the legacy mangled `void_.class` reappears, proving the fix is load-bearing.
	t.Setenv("JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "void_.class") {
		t.Errorf("fix OFF: expected legacy mangled `void_.class` (kill-switch not load-bearing), got:\n%s", off)
	}
}
