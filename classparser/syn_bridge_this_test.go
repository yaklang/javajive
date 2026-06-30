package javaclassparser

// 承重测试: 合成 access-bridge 构造器 `C(C$N marker)` 的空体补回 `this()` 委派
// (emitBridgeThisCall, kill-switch JDEC_SYN_BRIDGE_THIS_OFF)。
//
// 镜像 guava AbstractFuture$UnsafeAtomicHelper / $SynchronizedHelper /
// AggregateFutureState$SynchronizedAtomicHelper: javac 为「跨 nest 访问私有无参构造器」合成
// `Sub(Outer$1)` 桥, 其字节码即 `aload_0; invokespecial <init>:()V`(= `this()`)。反编译把该
// `this()` 剥成空体; 扁平成顶层单元后空体隐式 `super()` → 父类私有 `Base()` → javac
// "constructor Base ... has private access"。修复补回 `this();` 委派回同类私有无参构造器。
// kill-switch 置位恢复空体, 证明承重。

import (
	"os"
	"strings"
	"testing"
)

func TestSynBridgeThisIsLoadBearing(t *testing.T) {
	sub, err := os.ReadFile("testdata/regression/SynBridgeSeed$Sub.class")
	if err != nil {
		t.Fatalf("read SynBridgeSeed$Sub seed: %v", err)
	}
	// Resolver for the sibling units (Base / the $1 marker) so the flat unit decompiles fully. The
	// synthetic-bridge detection itself is descriptor-keyed and resolver-independent.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	// Fix ON (default): the synthetic bridge ctor body carries the explicit `this()` delegation.
	os.Unsetenv("JDEC_SYN_BRIDGE_THIS_OFF")
	on, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "SynBridgeSeed$1 var1) {") || !bridgeBodyHasThis(on) {
		t.Errorf("fix ON: expected the synthetic bridge ctor to delegate `this();`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the bridge body is left empty -- the exact "has private access" recompile
	// blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_SYN_BRIDGE_THIS_OFF", "1")
	off, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if bridgeBodyHasThis(off) {
		t.Errorf("fix OFF: bridge ctor must NOT carry `this();` (expected empty body), got:\n%s", off)
	}
}

// bridgeBodyHasThis reports whether the synthetic bridge ctor `...SynBridgeSeed$1 var1) {` is followed
// by a `this();` delegation before its closing brace.
func bridgeBodyHasThis(src string) bool {
	i := strings.Index(src, "SynBridgeSeed$1 var1) {")
	if i < 0 {
		return false
	}
	rest := src[i:]
	end := strings.Index(rest, "}")
	if end < 0 {
		return false
	}
	return strings.Contains(rest[:end], "this();")
}
