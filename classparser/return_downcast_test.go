package javaclassparser

// 承重测试: 当方法声明返回具体引用类型, 但返回点的值静态类型是被擦除的 Object (泛型擦除 /
// try-with-resources 的 null-only 槽) 时, 必须发出合法的向下造型 `return (T) (v);`, 否则 javac
// 报 "Object cannot be converted to T", 是 fastjson2 JSON.parseObject 家族整树重编译的真实阻断
// (21 个错误)。kill-switch JDEC_OBJECT_RET_DOWNCAST_OFF 关掉治本后造型应消失、裸 `return v;`
// 复现, 证明这条治本是承重的。种子 = 合成的 `static String first(String)`。

import (
	"os"
	"regexp"
	"testing"
)

// objectDowncastRe matches a return whose value is downcast to the concrete reference return type,
// e.g. `return (String) (var2);`. The legacy (broken) form is a bare `return var2;`.
var objectDowncastRe = regexp.MustCompile(`return \(String\) \(var\d+\);`)

func TestReturnObjectDowncastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ReturnObjectDowncast.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the Object-typed null slot is returned with an explicit `(String)` downcast.
	os.Unsetenv("JDEC_OBJECT_RET_DOWNCAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !objectDowncastRe.MatchString(on) {
		t.Errorf("fix ON: expected a `return (String) (varN);` downcast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the downcast disappears (legacy uncast `return varN;` reappears),
	// proving this fix is what inserts it rather than some unrelated pass.
	t.Setenv("JDEC_OBJECT_RET_DOWNCAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if objectDowncastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the downcast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
