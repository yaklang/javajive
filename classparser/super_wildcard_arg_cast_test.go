package javaclassparser

// 承重测试: 下界通配符 `? super E` 消费者参数的实参造型 (ResolveInstantiatedParamType 放行 `? super X`
// 接收者实参 + substituteAndGateParam 取其下界为造型目标, kill-switch JDEC_GENERIC_SUPERWILDCARD_OFF)。
//
// `sink` 字段声明为 `SuperWildcardSink<? super E>`。调用 `sink.apply(x)` 时形参 T 绑定到捕获的 `? super E`,
// 这是消费者位置, 接受一个 E。源码把擦除后的 Object 实参造型成 `(E)`, 但泛型擦除把该造型从字节码抹去
// (字段的 apply 与实参都擦除为 Object, 不发 checkcast), 故反编译器必须重新补 `(E)` 才能重编译, 否则 javac 报
// "Object cannot be converted to CAP#1 (? super E)"。镜像 guava Collections2$FilteredCollection /
// Multisets$FilteredMultiset 的 `this.predicate.apply((E) element)`。
// 旧逻辑在 ResolveInstantiatedParamType 入口对**任意**通配符接收者实参一律 bail, 故造型缺失。
// kill-switch 置位后恢复一刀切 bail, 造型消失, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// superWildcardArgCastRe matches the `(E)` cast re-synthesized on the Object argument to a
// `? super E`-typed consumer call, e.g. `this.sink.apply((E)(var1))`.
var superWildcardArgCastRe = regexp.MustCompile(`apply\(\(E\)\(`)

func TestSuperWildcardArgCastIsLoadBearing(t *testing.T) {
	names := []string{"SuperWildcardSink", "SuperWildcardSeed"}
	bytesByName := map[string][]byte{}
	for _, n := range names {
		data, err := os.ReadFile("testdata/regression/" + n + ".class")
		if err != nil {
			t.Fatalf("read seed %s: %v", n, err)
		}
		bytesByName[n] = data
	}
	// Resolver feeds every sibling seed's bytes by binary internal name (default package -> bare name),
	// so the cross-class walk can read SuperWildcardSink's generic signature (`apply(T)`).
	resolver := func(internalName string) ([]byte, bool) {
		data, ok := bytesByName[internalName]
		return data, ok
	}
	implBytes := bytesByName["SuperWildcardSeed"]

	// Fix ON (default): the `? super E` receiver arg is allowed through the resolver, the apply param
	// resolves to its lower bound E, and the erased Object argument is cast `(E)`.
	os.Unsetenv("JDEC_GENERIC_SUPERWILDCARD_OFF")
	os.Unsetenv("JDEC_GENERIC_RESOLVE_OFF")
	on, err := DecompileWithResolver(implBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !superWildcardArgCastRe.MatchString(on) {
		t.Errorf("fix ON: expected `apply((E)(...))` consumer cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): restore the blanket bail on any wildcard receiver arg -> the `(E)` cast
	// disappears (the exact "Object cannot be converted to CAP#" recompile blocker), proving it is
	// load-bearing.
	t.Setenv("JDEC_GENERIC_SUPERWILDCARD_OFF", "1")
	off, err := DecompileWithResolver(implBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if superWildcardArgCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the `(E)` consumer cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
