package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestSourceConstructorDiamondRequiresFunctionalOperandAndOriginalSignature(t *testing.T) {
	for _, variant := range []string{"lambda", "ordinary", "unknown", "nongeneric", "disabled", "missing call", "nil node", "nil context", "nil type"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			ctx := &class_context.ClassContext{Env: func(string) string { return "" }, SiblingClassSig: func(name string) (string, map[string]string, bool) {
				calls++
				if name != "example/Box" {
					t.Fatalf("foreign class %s", name)
				}
				if variant == "unknown" {
					return "", nil, false
				}
				if variant == "nongeneric" {
					return "Ljava/lang/Object;", nil, true
				}
				return "<T:Ljava/io/Serializable;>Ljava/lang/Object;", nil, true
			}}
			node := &NewExpression{JavaType: types.NewJavaClass("example.Box"), ConstructorCall: &FunctionCallExpression{Arguments: []JavaValue{&CustomValue{Flag: "lambda"}}}}
			switch variant {
			case "ordinary":
				node.ConstructorCall.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.io.Serializable"))}
			case "disabled":
				ctx.Env = func(string) string { return "1" }
			case "missing call":
				node.ConstructorCall = nil
			case "nil node":
				node = nil
			case "nil context":
				ctx = nil
			case "nil type":
				node.JavaType = nil
			}
			want := ""
			if variant == "lambda" {
				want = "<>"
			}
			if got := node.SourceConstructorDiamond(ctx); got != want {
				t.Fatalf("diamond %q want %q", got, want)
			}
			if variant != "lambda" && variant != "unknown" && variant != "nongeneric" && calls != 0 {
				t.Fatalf("unnecessary resolver call: %d", calls)
			}
		})
	}
}
