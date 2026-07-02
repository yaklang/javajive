package javaclassparser

// 承重测试: 泛型方法声明返回 Map<K,List<V>>, 方法体返回 var0.asMap(), 其中 var0:ListLike<K,V>,
// ListLike 继承 MapLike<K,V>, MapLike 声明 Map<K,Collection<V>> asMap()。故 asMap() 真实泛型返回是
// Map<K,Collection<V>>, 与声明返回同 raw 擦除(Map)但实参不同(不变型): 直接造型 inconvertible,
// 唯 raw 桥接 (Map<K,List<V>>)(Map)value 合法。字节码里 checkcast 到 Map 均 no-op 被丢弃, 反编译得裸
// return。治法(JDEC_PARAM_RETURN_RAW_BRIDGE_OFF): 经接收者层级 SiblingClassSig 恢复 asMap 的真实实例化
// 泛型返回, 判定同擦除异参后重新发出桥接造型。镜像 guava Multimaps.asMap(ListMultimap)。
// 需 MapLike/ListLike 兄弟单元由 resolver 提供(单类反编译无 SiblingClassSig, 治本不触发)。

import (
	"os"
	"strings"
	"testing"
)

func TestParameterizedReturnRawBridgeIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/ParamReturnRawBridgeSeed.class")
	if err != nil {
		t.Fatalf("read ParamReturnRawBridgeSeed seed: %v", err)
	}
	// Resolver for the sibling MapLike/ListLike units so the receiver's asMap() TRUE generic return
	// (Map<K,Collection<V>>, inherited from MapLike) is recoverable; the single class alone cannot see it.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_PARAM_RETURN_RAW_BRIDGE_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (bridge ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Map<K, List<V>>) (Map) (var0.asMap())") {
		t.Errorf("bridge ON: expected raw bridge `(Map<K, List<V>>) (Map) (var0.asMap())`, got:\n%s", on)
	}

	t.Setenv("JDEC_PARAM_RETURN_RAW_BRIDGE_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (bridge OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Map) (var0.asMap())") {
		t.Errorf("bridge OFF: expected the raw bridge to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
