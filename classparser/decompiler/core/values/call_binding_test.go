package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
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

func TestUniqueRawGenericCallNeedsNoErasureCast(t *testing.T) {
	ctx := &class_context.ClassContext{
		InvocationMetadata: func(name string) (callbinding.Class, bool) {
			if name == "example/List" {
				return callbinding.Class{
					Name: "example/List", Public: true, IsInterface: true,
					MembersComplete: true, ParentsComplete: true,
					Methods: []callbinding.Method{{
						Name: "add", Desc: "(Ljava/lang/Object;)Z", Public: true, Generic: true,
					}},
				}, true
			}
			return callbinding.Class{}, false
		},
	}
	id := utils.NewRootVariableId()
	id.SetName("list")
	receiver := NewJavaRef(id, nil, types.NewJavaClass("example.List"))
	methodType, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Z")
	if err != nil {
		t.Fatal(err)
	}
	call := &FunctionCallExpression{
		Object: receiver, ClassName: "example.List", FunctionName: "add",
		Descriptor: "(Ljava/lang/Object;)Z", Kind: InvokeInterface, FuncType: methodType.FunctionType(),
		Arguments: []JavaValue{NewJavaLiteral("first", types.NewJavaClass("java.lang.String"))},
	}
	if got := call.String(ctx); got != `list.add("first")` {
		t.Fatalf("raw unique generic call rendering %q", got)
	}
	if ctx.OverloadFamilyUnproven {
		t.Fatal("a complete unique erased binding must not be reported unsupported")
	}
}
