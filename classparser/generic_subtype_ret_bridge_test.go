package javaclassparser

// 承重测试: 泛型方法返回 `X<C>`(C 为作用域类型变量), 方法体返回一个 X 的 final 非泛型子类(经静态工厂
// 调用取得)。该子类固定超类型实参为 `X<Comparable>`, 直接 `(X<C>)` 造型 inconvertible、非受检转换也不适用,
// 唯有 raw 桥接 `(X<C>)(X)value` 合法; areturn 校验保证 value <: X 故永不 inconvertible。字节码里向超类
// upcast 不发 checkcast, 反编译得裸 `return get();`。治法(JDEC_GENERIC_SUBTYPE_RET_BRIDGE_OFF): 经子类
// SiblingClassSig 判定其为非泛型子类并重新发出桥接造型。镜像 guava Cut.aboveAll()/belowAll()。
// 种子 SubtypeRetBridgeSeed.top() 返回 SubtypeRetBridgeBase<C>, 体 `return SubtypeRetBridgeTop.get();`,
// 需 Base/Top 兄弟单元由 resolver 提供(单类反编译无 SiblingClassSig, 治本不触发)。

import (
	"os"
	"strings"
	"testing"
)

func TestGenericSubtypeReturnBridgeIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/SubtypeRetBridgeSeed.class")
	if err != nil {
		t.Fatalf("read SubtypeRetBridgeSeed seed: %v", err)
	}
	// Resolver for the sibling Base/Top units so the returned value's class Signature (non-generic
	// subtype fixing the supertype's type arg) is resolvable; the single class alone cannot see them.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_GENERIC_SUBTYPE_RET_BRIDGE_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (bridge ON) failed: %v", err)
	}
	if !strings.Contains(on, "(SubtypeRetBridgeBase<C>) (SubtypeRetBridgeBase) (SubtypeRetBridgeTop.get())") {
		t.Errorf("bridge ON: expected raw bridge `(SubtypeRetBridgeBase<C>) (SubtypeRetBridgeBase) (SubtypeRetBridgeTop.get())`, got:\n%s", on)
	}

	t.Setenv("JDEC_GENERIC_SUBTYPE_RET_BRIDGE_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (bridge OFF) failed: %v", err)
	}
	if strings.Contains(off, "(SubtypeRetBridgeBase) (SubtypeRetBridgeTop.get())") {
		t.Errorf("bridge OFF: expected the raw bridge to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
