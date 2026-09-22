package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestBindingCastSurvivesIdentityFoldAndInvokeClone(t *testing.T) {
	v := NewJavaLiteral("x", types.NewJavaClass("java.lang.String"))
	c := &CastExpression{Value: v, TargetType: v.Type(), OriginPC: 17, Binding: true}
	if FoldIdentityCast(c) != c {
		t.Fatal("binding cast erased")
	}
	f := &FunctionCallExpression{ClassName: "Base", FunctionName: "pick", Descriptor: "(Ljava/lang/String;)I", Kind: InvokeVirtual, OriginPC: 19, Arguments: []JavaValue{c}}
	clone := f.Clone()
	if clone.Witness() != f.Witness() || clone.Arguments[0] != c {
		t.Fatal("clone lost binding or witness")
	}
	if got := clone.renderArgAt(0, &class_context.ClassContext{}); got != "(String)(\"x\")" {
		t.Fatalf("binding render %q", got)
	}
}
