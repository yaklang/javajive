package javaclassparser

// 承重测试: 注解值里的外部嵌套枚举常量, 渲染为点号源名 `AnnoEnumAnno.Kind.FULL` 而非扁平二进制名
// `AnnoEnumAnno$Kind.FULL`(externalNestedEnumSourceName, kill-switch JDEC_ANNO_ENUM_NESTED_DOT_OFF)。
//
// 镜像 guava `@ReflectionSupport(value=ReflectionSupport$Level.FULL)`(AbstractFuture /
// AggregateFutureState / InterruptibleTask): 注解值是嵌套枚举常量, 字节码存其二进制描述符
// `LAnnoEnumAnno$Kind;`。扁平 `$` 名在 Java 源码里不可解析, javac 报 "an enum annotation value must be
// an enum constant"(并连带非法的 `import ...$Kind;`)。当枚举宿主**不是**反编译同胞单元
// (foldSiblingResolver miss = 外部依赖)时, 改写成点号源名 `AnnoEnumAnno.Kind.FULL`。
//
// 用「永远 miss 的 resolver」模拟外部依赖(种子已删除 AnnoEnumAnno$Kind.class)。kill-switch 置位后
// 回退扁平名, 证明承重。

import (
	"os"
	"strings"
	"testing"
)

func TestAnnoEnumNestedDotIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/AnnoEnumSeed.class")
	if err != nil {
		t.Fatalf("read AnnoEnumSeed seed: %v", err)
	}
	// A resolver that resolves NOTHING -- every nested type is provably external (a decompiled sibling
	// would resolve here and keep its flat `$` name).
	extResolver := func(internalName string) ([]byte, bool) { return nil, false }

	// Fix ON (default): external nested enum value rendered with the dotted source name.
	os.Unsetenv("JDEC_ANNO_ENUM_NESTED_DOT_OFF")
	on, err := DecompileWithResolver(seed, extResolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "AnnoEnumAnno.Kind.FULL") {
		t.Errorf("fix ON: expected dotted `AnnoEnumAnno.Kind.FULL`, got:\n%s", on)
	}
	if strings.Contains(on, "AnnoEnumAnno$Kind") {
		t.Errorf("fix ON: must NOT emit flat `AnnoEnumAnno$Kind`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the flat binary `$` name returns -- the exact unresolvable form javac
	// rejects ("an enum annotation value must be an enum constant"), proving the fix is load-bearing.
	t.Setenv("JDEC_ANNO_ENUM_NESTED_DOT_OFF", "1")
	off, err := DecompileWithResolver(seed, extResolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "AnnoEnumAnno$Kind.FULL") {
		t.Errorf("fix OFF: expected the flat `AnnoEnumAnno$Kind.FULL` fallback, got:\n%s", off)
	}
}
