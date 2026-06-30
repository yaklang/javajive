package javaclassparser

// 承重测试: 传给 JDK 泛型方法 (Map.put(K,V) / BiConsumer.accept(T,V) / Collection.add(E) ...) 的实参,
// 当其形参被字节码描述符擦除为 Object 时, 必须按接收者的参数化类型还原出源码原有的 `(K)`/`(V)` 造型,
// 否则 javac 报 "Object cannot be converted to K" —— fastjson2 FieldReaderBigDecimalFunc / guava
// Map.put 家族的整树重编译阻断。kill-switch JDEC_GENERIC_PARAM_INFER_OFF 关掉后造型消失, 证明承重。
// 种子 = 合成的 `GenArgCastSeed<K,V>.copy(Map<K,V>, Iterator)` 里的 `dst.put((K)e,(V)e)`。

import (
	"os"
	"regexp"
	"testing"
)

// genericArgCastRe matches the re-synthesized `(K)(varN),(V)(varN)` argument casts in a Map.put call.
var genericArgCastRe = regexp.MustCompile(`\.put\(\(K\)\(var\d+\),\(V\)\(var\d+\)\);`)

func TestGenericArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GenericArgCast.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the Object args are cast to the receiver's K / V type args.
	os.Unsetenv("JDEC_GENERIC_PARAM_INFER_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !genericArgCastRe.MatchString(on) {
		t.Errorf("fix ON: expected `put((K)(varN),(V)(varN))` casts, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the casts disappear (legacy uncast `put(varN,varN)`), proving this fix
	// is what re-synthesizes them rather than some unrelated pass.
	t.Setenv("JDEC_GENERIC_PARAM_INFER_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if genericArgCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the arg casts to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
