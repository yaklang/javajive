package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestSourceAnonymousCandidateDoesNotRenderUnownedOperands(t *testing.T) {
	rendered, attempted := 0, 0
	ctx := &class_context.ClassContext{ClassName: "Owner", SourceAnonymousCandidate: func(string) bool { return false }, SourceAnonymousAllocation: func(string, string, int, int, []class_context.SourceCaptureOperand) (string, bool) {
		attempted++
		return "", false
	}}
	arg := NewCustomValue(func(*class_context.ClassContext) string { rendered++; return "payload" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
	typ, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)V")
	if err != nil {
		t.Fatal(err)
	}
	call := &FunctionCallExpression{ClassName: "Unowned", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", Arguments: []JavaValue{arg}, FuncType: typ.FunctionType(), Kind: InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 7}
	allocation := &NewExpression{JavaType: types.NewJavaClass("Unowned"), ConstructorCall: call, HasOriginPC: true, OriginPC: 3, ArgumentsGetter: func() string { return arg.String(ctx) }}
	_ = allocation.String(ctx)
	if rendered != 1 || attempted != 0 {
		t.Fatalf("unowned operand rendered %d times; native attempts %d", rendered, attempted)
	}
}

func TestSourceMemberCandidateDoesNotRenderUnownedOperands(t *testing.T) {
	rendered, attempted := 0, 0
	ctx := &class_context.ClassContext{ClassName: "Owner", SourceMemberCandidate: func(string) bool { return false }, SourceMemberAllocation: func(string, string, int, int, []class_context.SourceCaptureOperand) (string, bool) {
		attempted++
		return "", false
	}}
	arg := NewCustomValue(func(*class_context.ClassContext) string { rendered++; return "payload" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
	typ, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)V")
	if err != nil {
		t.Fatal(err)
	}
	call := &FunctionCallExpression{ClassName: "Unowned", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", Arguments: []JavaValue{arg}, FuncType: typ.FunctionType(), Kind: InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 7}
	allocation := &NewExpression{JavaType: types.NewJavaClass("Unowned"), ConstructorCall: call, HasOriginPC: true, OriginPC: 3, ArgumentsGetter: func() string { return arg.String(ctx) }}
	_ = allocation.String(ctx)
	if rendered != 1 || attempted != 0 {
		t.Fatalf("unowned operand rendered %d times; native attempts %d", rendered, attempted)
	}
}

func TestSourceMemberConstructorGateRunsBeforeOperandRendering(t *testing.T) {
	rendered, attempted, selected := 0, 0, 0
	ctx := &class_context.ClassContext{ClassName: "Owner", SourceMemberCandidate: func(string) bool { return true }, SourceMemberDescriptorCandidate: func(owner, desc string) bool {
		selected++
		if owner != "Owned" || desc != "(Ljava/lang/Object;)V" {
			t.Fatalf("original constructor identity %s %s", owner, desc)
		}
		return false
	}, SourceMemberAllocation: func(string, string, int, int, []class_context.SourceCaptureOperand) (string, bool) {
		attempted++
		return "", false
	}}
	arg := NewCustomValue(func(*class_context.ClassContext) string { rendered++; return "payload" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
	typ, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)V")
	if err != nil {
		t.Fatal(err)
	}
	call := &FunctionCallExpression{ClassName: "Owned", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", Arguments: []JavaValue{arg}, FuncType: typ.FunctionType(), Kind: InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 7}
	allocation := &NewExpression{JavaType: types.NewJavaClass("Owned"), ConstructorCall: call, HasOriginPC: true, OriginPC: 3, ArgumentsGetter: func() string { return arg.String(ctx) }}
	_ = allocation.String(ctx)
	if rendered != 1 || attempted != 0 || selected != 1 {
		t.Fatalf("renders=%d projections=%d selections=%d", rendered, attempted, selected)
	}
}
