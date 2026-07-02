package javaclassparser

// 承重测试: 泛型类 X<C> 的方法 make 先 `C next = domain.next(this.endpoint);` 再 `return of(next);`
// (of 是 `<C> Box<C> of(C)`)。domain.next(C) 真实返回类型变量 C, 但字节码擦除到边界 Comparable 存入
// 局部槽, 反编译器据存值静态类型会把局部声明成 Comparable; 于是 of(next) 推断 Box<Comparable> 与声明
// 返回 Box<C> 不变型冲突, 不可编译。治法(JDEC_TYPEVAR_LOCAL_DECL_OFF): 经 SiblingClassSig 恢复 next 的
// 真实实例化返回是作用域内类型变量 C, 且调用非 unchecked, 遂把局部声明治本为 `C next`。镜像 guava
// Cut$AboveValue.withLowerBoundType。需 Domain 兄弟单元由 resolver 提供 (单类反编译无 SiblingClassSig,
// 治本不触发)。

import (
	"os"
	"strings"
	"testing"
)

func TestTypeVarLocalDeclIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/TypeVarLocalDeclSeed.class")
	if err != nil {
		t.Fatalf("read TypeVarLocalDeclSeed seed: %v", err)
	}
	// Resolver for the sibling Domain unit so next()'s TRUE generic return (the class type variable C)
	// is recoverable; the single class alone cannot see it (SiblingClassSig is nil for single-class).
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_TYPEVAR_LOCAL_DECL_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "C var2 = var1.next(") {
		t.Errorf("fix ON: expected local declared at the type variable `C var2 = var1.next(...)`, got:\n%s", on)
	}

	t.Setenv("JDEC_TYPEVAR_LOCAL_DECL_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "Comparable var2 = var1.next(") {
		t.Errorf("fix OFF: expected the local to fall back to the erased bound `Comparable var2 = var1.next(...)` (kill-switch not load-bearing), got:\n%s", off)
	}
}
