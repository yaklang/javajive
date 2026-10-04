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
	"strings"
	"testing"
)

func TestSynBridgeThisIsLoadBearing(t *testing.T) {
	path := "testdata/regression/SynBridgeSeed$Sub.class"
	assertReviewedTypeVarInvoke(t, path, "<init>", "(LSynBridgeSeed$1;)V", 1, 183, "SynBridgeSeed$Sub", "<init>", "()V")
	assertReviewedTypeVarInvoke(t, path, "<init>", "()V", 2, 183, "SynBridgeSeed$Base", "<init>", "(LSynBridgeSeed$1;)V")
	reviewedSeedSources(t, path, "JDEC_SYN_BRIDGE_THIS_OFF", true, func(source string) {
		requireReviewedPattern(t, source, `SynBridgeSeed\$Sub\(SynBridgeSeed\$1\s+\w+\)\s*\{\s*this\(\);\s*\}`)
		requireReviewedPattern(t, source, `SynBridgeSeed\$Sub\(\)\s*\{\s*super\(\(SynBridgeSeed\$1\)\(null\)\);\s*\}`)
	})
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
