package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestConstructorOperandsUseCurrentRenderContext(t *testing.T) {
	for _, scope := range []string{"Caller", "Initializer", "Preview"} {
		t.Run(scope, func(t *testing.T) {
			stale, current := 0, 0
			ctx := &class_context.ClassContext{ClassName: scope}
			arg := NewCustomValue(func(got *class_context.ClassContext) string {
				current++
				if got != ctx {
					t.Fatal("operand rendered in stale context")
				}
				return "payload"
			}, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			mt, e := types.ParseMethodDescriptor("(Ljava/lang/Object;)V")
			if e != nil {
				t.Fatal(e)
			}
			call := &FunctionCallExpression{ClassName: "Owner", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", Arguments: []JavaValue{arg}, FuncType: mt.FunctionType(), Kind: InvokeSpecial, IsSpecialInvoke: true}
			n := &NewExpression{JavaType: types.NewJavaClass("Owner"), ConstructorCall: call, ArgumentsGetter: func() string { stale++; return "stale" }}
			if got := n.String(ctx); got != "new Owner(payload)" {
				t.Fatalf("source=%q", got)
			}
			if stale != 0 || current != 1 {
				t.Fatalf("stale=%d current=%d", stale, current)
			}
		})
	}
}
func TestLegacyConstructorGetterRemainsFallback(t *testing.T) {
	calls := 0
	n := &NewExpression{JavaType: types.NewJavaClass("Owner"), ArgumentsGetter: func() string { calls++; return "legacy" }}
	if got := n.String(&class_context.ClassContext{}); !strings.HasSuffix(got, "(legacy)") || calls != 1 {
		t.Fatalf("legacy source=%q calls=%d", got, calls)
	}
}
