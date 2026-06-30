package javaclassparser

// 承重测试: 对**继承自直接超类型**的泛型方法在 `this` 上的调用 (`this.grab(objVal)`, 方法声明在接口
// SuperSeed<K,V> 而非本类), 必须沿 jar 内直接超类型链还原其形参类型, 给实参补回源码原有的 `(K)` 造型
// (guava AbstractLoadingCache `this.get(k)` 来自接口 LoadingCache<K,V> 的家族)。
// 实现要点: 反编译本类时, 用 foldSiblingResolver 加载直接超类型字节, 在**恒等类型实参映射**
// (`Sub<K,V> implements Super<K,V>`) 下把超类型方法的泛型 Signature 并入本类 MethodSignatures。
// 安全边界: 仅恒等映射 + 仅类作用域类型变量造型; 换序/具体实参的非恒等映射保守跳过。
// 关键: 该治本依赖跨类 resolver, 故必须用 DecompileWithResolver(两类: 接口 + 实现), 单类 Decompile 不触发。
// kill-switch JDEC_GENERIC_SUPER_METHOD_OFF 关掉后造型消失, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// inheritedThisMethodArgCastRe matches the `(K)` cast re-synthesized on the argument to an inherited
// `this.grab(...)` call, e.g. `this.grab((K)(var1))`.
var inheritedThisMethodArgCastRe = regexp.MustCompile(`this\.grab\(\(K\)\(`)

func TestInheritedThisMethodArgCastIsLoadBearing(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatalf("read impl seed: %v", err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatalf("read super seed: %v", err)
	}
	// Resolver feeds the direct supertype's bytes by binary internal name (default package -> bare name).
	resolver := func(internalName string) ([]byte, bool) {
		if internalName == "SuperSeed" {
			return superBytes, true
		}
		return nil, false
	}

	// Fix ON (default): the inherited interface method's K parameter is recovered across the type chain,
	// so the Object argument is cast to K.
	os.Unsetenv("JDEC_GENERIC_SUPER_METHOD_OFF")
	on, err := DecompileWithResolver(implBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !inheritedThisMethodArgCastRe.MatchString(on) {
		t.Errorf("fix ON: expected a `(K)` cast on the inherited `this.grab` arg, got:\n%s", on)
	}

	// Fix OFF: the cross-supertype signature augmentation is disabled, so the cast disappears, proving
	// the supertype-chain lookup (not some unrelated pass) is what re-synthesizes it.
	t.Setenv("JDEC_GENERIC_SUPER_METHOD_OFF", "1")
	off, err := DecompileWithResolver(implBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if inheritedThisMethodArgCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the inherited arg cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
