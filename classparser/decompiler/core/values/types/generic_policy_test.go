package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/jdecenv"
)

func TestJDKParamResolutionRetainsExplicitPolicyAtHierarchyLeaf(t *testing.T) {
	for _, gate := range []string{"JDEC_GENERIC_PARAM_INFER_OFF", "JDEC_SORTED_MAP_KEY_PARAM_OFF", "JDEC_DEQUE_PARAM_OFF", "JDEC_LIST_SET_PARAM_OFF", "JDEC_ATOMIC_REF_PARAM_OFF", "JDEC_GENERIC_SUPERWILDCARD_OFF"} {
		t.Run(gate, func(t *testing.T) {
			owner, method, argc, index := "java.util.List", "add", 1, 0
			args := []JavaType{NewJavaClass("java.lang.String")}
			switch gate {
			case "JDEC_SORTED_MAP_KEY_PARAM_OFF":
				owner, method = "java.util.SortedMap", "headMap"
				args = append(args, NewJavaClass("java.lang.Integer"))
			case "JDEC_DEQUE_PARAM_OFF":
				owner, method = "java.util.Deque", "push"
			case "JDEC_LIST_SET_PARAM_OFF":
				method, argc, index = "set", 2, 1
			case "JDEC_ATOMIC_REF_PARAM_OFF":
				owner, method = "java.util.concurrent.atomic.AtomicReference", "set"
			case "JDEC_GENERIC_SUPERWILDCARD_OFF":
				args[0] = &JavaWildcardType{Variant: "super", Bound: args[0]}
			}
			provider := func(string) (string, map[string]string, bool) { return "", nil, false }
			for _, explicitOff := range []bool{false, true} {
				policy := func(key string) string {
					if explicitOff && key == gate {
						return "1"
					}
					return ""
				}
				ambient := map[string]string{}
				if !explicitOff {
					ambient[gate] = "1"
				}
				_ = jdecenv.Run(ambient, func() error {
					ctx := &class_context.ClassContext{Env: policy}
					got := ResolveInstantiatedParamType(ctx, provider, owner, args, method, "", argc, index)
					if (got == nil) != explicitOff {
						t.Fatalf("explicit off=%v resolved=%v", explicitOff, got)
					}
					return nil
				})
			}
		})
	}
}

func TestNestedJDKFormalUsesExplicitPolicy(t *testing.T) {
	args := []JavaType{NewJavaClass("java.lang.String"), NewJavaClass("java.lang.Integer")}
	closed := func(string) string { return "" }
	_ = jdecenv.Run(map[string]string{"JDEC_GENERIC_PARAM_INFER_OFF": "1"}, func() error {
		if got := InstantiateJDKMethodParamTypeWithEnv("java.util.Map", "compute", 2, 1, args, closed); got == nil {
			t.Fatal("ambient policy overrode explicit nested formal")
		}
		if got := InstantiateJDKMethodParamType("java.util.Map", "compute", 2, 1, args); got != nil {
			t.Fatal("legacy entry ignored its ambient policy")
		}
		return nil
	})
}

func BenchmarkJDKParamPolicy(b *testing.B) {
	args := []JavaType{NewJavaClass("java.lang.String")}
	closed := func(string) string { return "" }
	for _, explicit := range []bool{false, true} {
		name := "ambient"
		if explicit {
			name = "explicit"
		}
		b.Run(name, func(b *testing.B) {
			_ = jdecenv.Run(map[string]string{}, func() error {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if explicit {
						InstantiateJDKMethodParamInfoWithEnv("java.util.List", "add", 1, 0, args, closed)
					} else {
						InstantiateJDKMethodParamInfo("java.util.List", "add", 1, 0, args)
					}
				}
				return nil
			})
		})
	}
}
