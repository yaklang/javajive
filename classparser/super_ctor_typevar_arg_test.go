package javaclassparser

// 承重测试: `super(...)` 实参喂给超类构造器的裸型变形参时, 需从超类构造器签名 + 子类 extends 子句恢复
// 被擦除的 `(N)` 造型。
//
// 镜像 guava graph IncidentEdgeSet 子类 `super(graph, node)`(node 形参是 `TN`)与 RegularContiguousSet
// 匿名迭代器 `super(first)`(形参 `TC`): 源码传型变类型的值无造型, 字节码把值擦成 Object/上界且不发
// checkcast, 反编译渲染裸 `super(..., objVal)`, javac 按超类构造器 `(..., N)` 签名复解析报
// `Object cannot be converted to N/CAP#1`。治法(kill-switch JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF): 经
// SiblingCtorSig 取超类构造器泛型签名识别裸型变形参, 经子类 ClassSig 的 extends 子句把超类形参名映射到
// 子类自身作用域型变, 重新发出 `(N)` 造型(unchecked、行为等价)。种子 = 匿名子类 SuperCtorTypeVarSeed$1
// (extends SuperCtorTypeVarBaseSeed<N>, 构造器 `super("t", (N) node)`), 需 base 作为兄弟单元由 resolver 提供。

import (
	"os"
	"strings"
	"testing"
)

func TestSuperCtorTypeVarArgCastIsLoadBearing(t *testing.T) {
	sub, err := os.ReadFile("testdata/regression/SuperCtorTypeVarSeed$1.class")
	if err != nil {
		t.Fatalf("read SuperCtorTypeVarSeed$1 seed: %v", err)
	}
	// Resolver for the sibling base unit so the super ctor Signature (`(String, N)`) and the super class's
	// formal type-param names are resolvable; the flattened anon unit alone cannot see them.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	// Fix ON (default): the erased `(N)` cast on the super() type-variable argument is re-emitted.
	os.Unsetenv("JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF")
	on, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(N)(var3))") {
		t.Errorf("fix ON: expected super() arg cast `(N)(var3)`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the bare `super(var2,var3)` returns -- the exact "Object cannot be converted
	// to N" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF", "1")
	off, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(N)(var3))") {
		t.Errorf("fix OFF: expected the `(N)` cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
