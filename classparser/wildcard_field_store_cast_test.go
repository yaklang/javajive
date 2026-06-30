package javaclassparser

// 承重测试: 通配符参数化字段存储造型。同类字段声明为 `Class<? super T>` (通配符 + 类型变量 T),
// 构造器里 `this.rawType = raw(this.type)`, raw() 真实返回 `Class<?>`。调用点字节码只剩擦除后的裸
// `Class`, JavaJive 看不到冲突; 但 javac 用 raw() 真实签名 `Class<?>` 捕获成 CAP#1, 报
// `Class<CAP#1> cannot be converted to Class<? super T>`。真源码因此带 unchecked `(Class<? super T>)`
// 造型 (gson TypeToken `this.rawType = $Gson$Types.getRawType(this.type)`)。
// 实现要点: AssignStatement 字段存储路径用 funcCtx.FieldSignature 复原字段的参数化声明类型, 若其为
// 含通配符且提及类型变量的同擦除参数化, 且 RHS 擦除到同一裸类型, 补字段声明类型造型。
// kill-switch JDEC_WILDCARD_FIELD_CAST_OFF 置位后回退到裸存储, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// wildcardFieldCastRe matches the recovered unchecked cast on the wildcard-parameterized field store.
var wildcardFieldCastRe = regexp.MustCompile(`this\.rawType = \(Class<\? super T>\) \(raw\(this\.type\)\)`)

// wildcardFieldBareRe matches the bare (uncast) field store (the OFF / legacy emission).
var wildcardFieldBareRe = regexp.MustCompile(`this\.rawType = raw\(this\.type\);`)

func TestWildcardFieldStoreCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardFieldSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the wildcard-parameterized field store gets the recovered `(Class<? super T>)`
	// cast.
	os.Unsetenv("JDEC_WILDCARD_FIELD_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !wildcardFieldCastRe.MatchString(on) {
		t.Errorf("fix ON: expected recovered `this.rawType = (Class<? super T>) (raw(this.type))`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears and the store falls back to the bare assignment,
	// proving this fix re-synthesizes the cast.
	t.Setenv("JDEC_WILDCARD_FIELD_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if wildcardFieldCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the recovered cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !wildcardFieldBareRe.MatchString(off) {
		t.Errorf("fix OFF: expected the bare `this.rawType = raw(this.type);` fallback, got:\n%s", off)
	}
}
