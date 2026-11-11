package values

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func lambdaBuilderArguments(count int) []JavaValue {
	out := make([]JavaValue, count)
	integer := types.NewJavaPrimer(types.JavaInteger)
	for i := range out {
		text := fmt.Sprintf("arg%d", i)
		out[i] = NewCustomValue(func(*class_context.ClassContext) string { return text }, func() types.JavaType { return integer })
	}
	return out
}

func TestLambdaFuncRefExactArgumentOutput(t *testing.T) {
	ref := &LambdaFuncRef{Id: 17, Arguments: lambdaBuilderArguments(3)}
	if got, want := ref.String(&class_context.ClassContext{}), "getLambda(17)(arg0,arg1,arg2,)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func BenchmarkLambdaFuncRefBuilder(b *testing.B) {
	ref := &LambdaFuncRef{Id: 17, Arguments: lambdaBuilderArguments(4096)}
	ctx := &class_context.ClassContext{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got := ref.String(ctx); len(got) == 0 {
			b.Fatal("empty output")
		}
	}
}
