package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

func TestRenderPolicyUsesExplicitContextBeforeAmbientSnapshot(t *testing.T) {
	const flag = "JDEC_CTOR_DIAMOND_OFF"
	for _, explicit := range []string{"", "1"} {
		ambient := "1"
		if explicit == "1" {
			ambient = ""
		}
		_ = jdecenv.Run(map[string]string{flag: ambient}, func() error {
			ctx := &class_context.ClassContext{Env: func(key string) string {
				if key == flag {
					return explicit
				}
				return ""
			}, SiblingClassSig: func(string) (string, map[string]string, bool) {
				return "<E:Ljava/lang/Object;>Ljava/lang/Object;", nil, true
			}}
			node := &NewExpression{JavaType: types.NewJavaClass("example.Holder"), ConstructorCall: &FunctionCallExpression{Arguments: []JavaValue{&CustomValue{Flag: "lambda"}}}}
			want := "<>"
			if explicit == "1" {
				want = ""
			}
			if got := node.genericCtorDiamond(ctx); got != want {
				t.Errorf("explicit policy %q overridden by ambient %q: got %q want %q", explicit, ambient, got, want)
			}
			return nil
		})
	}
}

func TestInvocationTypeKeepsOriginatingRequestPolicy(t *testing.T) {
	const flag = "JDEC_GENERIC_INFER_OFF"
	t.Setenv(flag, "1")
	create := func() *FunctionCallExpression {
		receiver := NewJavaRef(nil, nil, types.NewParameterizedType("java.util.Iterator", []types.JavaType{types.NewJavaClass("java.lang.String")}))
		return NewFunctionCallExpression(receiver, &JavaClassMember{Name: "java.util.Iterator", Member: "next"}, &types.JavaFuncType{ReturnType: types.NewJavaClass("java.lang.Object")})
	}
	var inferred, erased *FunctionCallExpression
	_ = jdecenv.Run(map[string]string{}, func() error { inferred = create(); return nil })
	_ = jdecenv.Run(map[string]string{flag: "1"}, func() error { erased = create(); return nil })
	name := func(call *FunctionCallExpression) string { return call.Type().RawType().(*types.JavaClass).Name }
	assertTypes := func() {
		t.Helper()
		if got := name(inferred); got != "java.lang.String" {
			t.Errorf("originating request enabled inference, got %q", got)
		}
		if got := name(erased); got != "java.lang.Object" {
			t.Errorf("originating request disabled inference, got %q", got)
		}
		if got := name(inferred.Clone()); got != "java.lang.String" {
			t.Errorf("cloned call lost request policy: %q", got)
		}
	}
	assertTypes()
	_ = jdecenv.Run(map[string]string{}, func() error { assertTypes(); return nil })
	live := create()
	t.Setenv(flag, "")
	assertTypes()
	if got := name(live); got != "java.lang.String" {
		t.Errorf("unbound legacy invocation must keep live lookup, got %q", got)
	}
	t.Setenv(flag, "1")
	if got := name(live); got != "java.lang.Object" {
		t.Errorf("live policy was frozen, got %q", got)
	}
}
