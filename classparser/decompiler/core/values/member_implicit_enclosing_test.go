package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

// An implicit enclosing operand has no independent source expression. The
// narrow projection may omit only that operand; other arguments still render
// in order and a failed certificate retains ordinary rendering.
func TestSourceMemberImplicitEnclosingDoesNotRenderHiddenOperand(t *testing.T) {
	for _, proved := range []bool{true, false} {
		name := "refused"
		if proved {
			name = "proved"
		}
		t.Run(name, func(t *testing.T) {
			trace := ""
			hidden := NewCustomValue(func(*class_context.ClassContext) string { trace += "H"; return "outer" }, func() types.JavaType { return types.NewJavaClass("Owner") })
			payload := NewCustomValue(func(*class_context.ClassContext) string { trace += "P"; return "payload" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			mt, e := types.ParseMethodDescriptor("(LOwner;Ljava/lang/Object;)V")
			if e != nil {
				t.Fatal(e)
			}
			call := &FunctionCallExpression{ClassName: "Owner$Entry", FunctionName: "<init>", Descriptor: "(LOwner;Ljava/lang/Object;)V", Arguments: []JavaValue{hidden, payload}, FuncType: mt.FunctionType(), Kind: InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 9}
			allocation := &NewExpression{JavaType: types.NewJavaClass(call.ClassName), ConstructorCall: call, HasOriginPC: true, OriginPC: 2}
			queries, projections := 0, 0
			ctx := &class_context.ClassContext{ClassName: "Caller", SourceMemberCandidate: func(string) bool { return true }}
			ctx.SourceMemberImplicitEnclosing = func(owner, descriptor string, newPC, pc int, value any) bool {
				queries++
				if owner != call.ClassName || descriptor != call.Descriptor || newPC != 2 || pc != 9 || value != hidden {
					t.Fatal("consumer identity changed")
				}
				return proved
			}
			ctx.SourceMemberAllocation = func(owner, descriptor string, newPC, pc int, args []class_context.SourceCaptureOperand) (string, bool) {
				projections++
				if len(args) != 2 || args[0].Value != hidden || args[1].Value != payload || args[1].Text != "payload" {
					t.Fatal("projected operands changed")
				}
				if proved && args[0].Text != "" || !proved && args[0].Text != "outer" {
					t.Fatal("hidden operand rendered without matching permission")
				}
				return "new Entry(payload)", true
			}
			if got := allocation.String(ctx); got != "new Entry(payload)" {
				t.Fatalf("source=%q", got)
			}
			want := "HP"
			if proved {
				want = "P"
			}
			if trace != want || queries != 1 || projections != 1 {
				t.Fatalf("trace=%q query=%d projection=%d", trace, queries, projections)
			}
		})
	}
}
