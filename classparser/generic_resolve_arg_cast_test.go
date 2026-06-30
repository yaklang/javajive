package javaclassparser

// 承重测试: 统一跨类泛型解析器 (types.ResolveInstantiatedParamType)。它取代「同类 / 恒等一层超类型」
// 等特例拼盘, 用「沿接收者泛型超类型层级 DFS + 逐边类型实参替换 σ」一次覆盖三类泛型擦除缺造型残余:
//   (i)  非-this 接收者: 调用目标是 JAR 内泛型类型字段 (`this.box.put(o)`, box=Box<E>), put(T)->E;
//   (ii) 非恒等映射: `Sub<X,Y> implements Pair<Y,X>` 换序, first(A)->Y;
//   (iii) 深链: 方法声明在祖接口 (DeepThisSeed implements GenB<K> extends GenA<K>), grab(K) 在 GenA。
// 三者都被旧路径漏掉 (JDK 表只认 JDK; 同类 this 路径要求方法在本类; 恒等一层只认直接超类型恒等映射),
// 故必须用 DecompileWithResolver 喂全部 sibling 字节走跨类解析。kill-switch JDEC_GENERIC_RESOLVE_OFF
// 置位后造型消失, 证明承重于该解析器 (而非其它 pass)。

import (
	"os"
	"regexp"
	"testing"
)

// genericResolveSeed names a synthetic scenario plus the impl class to decompile and the cast regex it
// must re-synthesize when the resolver is ON.
type genericResolveSeed struct {
	name    string // sub-test name
	impl    string // impl seed class (binary internal name; default package -> bare name)
	castRe  *regexp.Regexp
	comment string
}

func loadGenericResolveSeeds(t *testing.T) (map[string][]byte, []genericResolveSeed) {
	t.Helper()
	names := []string{"GenA", "GenB", "DeepThisSeed", "Pair", "SwapSeed", "Box", "FieldRecvSeed"}
	bytesByName := map[string][]byte{}
	for _, n := range names {
		data, err := os.ReadFile("testdata/regression/" + n + ".class")
		if err != nil {
			t.Fatalf("read seed %s: %v", n, err)
		}
		bytesByName[n] = data
	}
	seeds := []genericResolveSeed{
		{
			name:    "deep_chain_this",
			impl:    "DeepThisSeed",
			castRe:  regexp.MustCompile(`grab\(\(K\)\(`),
			comment: "grab(K) declared two levels up in GenA, reached via GenB",
		},
		{
			name:    "non_identity_reorder",
			impl:    "SwapSeed",
			castRe:  regexp.MustCompile(`first\(\(Y\)\(`),
			comment: "Pair<Y,X> reorders type args; first(A) resolves to Y",
		},
		{
			name:    "non_this_field_receiver",
			impl:    "FieldRecvSeed",
			castRe:  regexp.MustCompile(`put\(\(E\)\(`),
			comment: "receiver this.box is jar-internal Box<E>; put(T) resolves to E",
		},
	}
	return bytesByName, seeds
}

func TestGenericResolveArgCastIsLoadBearing(t *testing.T) {
	bytesByName, seeds := loadGenericResolveSeeds(t)
	// Resolver feeds every sibling seed's bytes by binary internal name (default package -> bare name).
	resolver := func(internalName string) ([]byte, bool) {
		data, ok := bytesByName[internalName]
		return data, ok
	}

	for _, s := range seeds {
		s := s
		t.Run(s.name, func(t *testing.T) {
			implBytes := bytesByName[s.impl]

			// Fix ON (default): the cross-class resolver recovers the callee's generic parameter type and
			// re-emits the erased cast on the Object argument.
			os.Unsetenv("JDEC_GENERIC_RESOLVE_OFF")
			on, err := DecompileWithResolver(implBytes, resolver)
			if err != nil {
				t.Fatalf("[%s] decompile (fix ON) failed: %v", s.name, err)
			}
			if !s.castRe.MatchString(on) {
				t.Errorf("[%s] fix ON: expected cast %q (%s), got:\n%s", s.name, s.castRe, s.comment, on)
			}

			// Fix OFF: disabling the unified resolver drops the cast, proving the resolver (not some other
			// pass) is what re-synthesizes it.
			t.Setenv("JDEC_GENERIC_RESOLVE_OFF", "1")
			off, err := DecompileWithResolver(implBytes, resolver)
			if err != nil {
				t.Fatalf("[%s] decompile (fix OFF) failed: %v", s.name, err)
			}
			if s.castRe.MatchString(off) {
				t.Errorf("[%s] fix OFF: expected cast %q to disappear (kill-switch not load-bearing), got:\n%s", s.name, s.castRe, off)
			}
		})
	}
}
