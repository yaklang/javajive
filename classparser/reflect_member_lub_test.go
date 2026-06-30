package javaclassparser

// 承重测试: 反射家族三元 LUB(两耦合修复)
//   1. 层级表反射行 (kill-switch JDEC_TYPELUB_OFF):
//      Method/Field/Constructor 的 use-correct 公共类型是接口 java.lang.reflect.Member(有
//      getName/getDeclaringClass), 而非类 AccessibleObject(没有这些方法)。表里只映射到 Member(刻意
//      省略 AccessibleObject), 使 `cond ? method : field` 的 LUB 为 Member。
//   2. 三元声明缓存类型刷新 (kill-switch JDEC_TERNARY_DECL_LUB_CACHE_OFF):
//      ternaryDeclLUB 之前只重置左值 ref, 但声明类型渲染取自 RHS 三元的 cachedType —— 该缓存可能是装配
//      期(臂未定型前)铸出的窄类型(Method)。补 SetCachedType 让 value.Type() 与加宽后的 ref 一致。
//
// 镜像 fastjson2 FieldReader.toString / compareTo `Member m = method != null ? method : field`。
// 两个 kill-switch 各自独立复现 `Method var1`(bad type in conditional), 分别证明承重。

import (
	"os"
	"strings"
	"testing"
)

func reflectMemberLUBDecompile(t *testing.T) string {
	t.Helper()
	seed, err := os.ReadFile("testdata/regression/ReflectMemberLUBSeed.class")
	if err != nil {
		t.Fatalf("read ReflectMemberLUBSeed seed: %v", err)
	}
	out, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	return out
}

func TestReflectMemberLUBTableIsLoadBearing(t *testing.T) {
	// Hold the cache-refresh ON so we observe ONLY the hierarchy-table reflection rows.
	os.Unsetenv("JDEC_TERNARY_DECL_LUB_CACHE_OFF")

	// Fix ON (default): the Method/Field merge resolves to the interface Member.
	os.Unsetenv("JDEC_TYPELUB_OFF")
	on := reflectMemberLUBDecompile(t)
	if !strings.Contains(on, "Member var1 = ") {
		t.Errorf("fix ON: expected `Member var1 = ` LUB declaration, got:\n%s", on)
	}

	// Fix OFF (kill-switch): MergeTypes falls back to the first arm (Method), reproducing the
	// `Method var1 = cond ? method : field` recompile blocker -- proving the table rows load-bearing.
	t.Setenv("JDEC_TYPELUB_OFF", "1")
	off := reflectMemberLUBDecompile(t)
	if strings.Contains(off, "Member var1 = ") || !strings.Contains(off, "Method var1 = ") {
		t.Errorf("fix OFF: expected first-arm fallback `Method var1 = `, got:\n%s", off)
	}
}

func TestTernaryDeclLUBCacheRefreshIsLoadBearing(t *testing.T) {
	// Hold the reflection rows ON so the LUB IS Member; observe ONLY the cache-refresh.
	os.Unsetenv("JDEC_TYPELUB_OFF")

	// Fix ON (default): the refreshed cache makes the declaration render the widened Member.
	os.Unsetenv("JDEC_TERNARY_DECL_LUB_CACHE_OFF")
	on := reflectMemberLUBDecompile(t)
	if !strings.Contains(on, "Member var1 = ") {
		t.Errorf("fix ON: expected `Member var1 = `, got:\n%s", on)
	}

	// Fix OFF (kill-switch): ternaryDeclLUB still widens the ref, but the declaration renders the
	// STALE cached first-arm type `Method` -- the exact symptom the refresh removes -- proving it
	// load-bearing even when the LUB itself is available.
	t.Setenv("JDEC_TERNARY_DECL_LUB_CACHE_OFF", "1")
	off := reflectMemberLUBDecompile(t)
	if strings.Contains(off, "Member var1 = ") || !strings.Contains(off, "Method var1 = ") {
		t.Errorf("fix OFF: expected stale `Method var1 = ` declaration, got:\n%s", off)
	}
}
