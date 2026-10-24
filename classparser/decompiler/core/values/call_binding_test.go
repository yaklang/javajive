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

func TestCheckedReceiverBindingKeepsProducersAndProofBoundaries(t *testing.T) {
	for _, variant := range []string{"class declaration", "interface declaration", "same owner", "missing original", "unrelated receiver", "inaccessible owner", "generic declaration", "bridge declaration", "wrong invoke kind"} {
		t.Run(variant, func(t *testing.T) {
			declaration := callbinding.Class{Name: "sample/Contract", Public: true, MembersComplete: true, ParentsComplete: true,
				Methods: []callbinding.Method{{Name: "invoke", Desc: "()V", Public: true, ExceptionsKnown: true, Exceptions: []string{"java/io/IOException"}}}}
			implementation := callbinding.Class{Name: "sample/Implementation", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{declaration.Name},
				Methods: []callbinding.Method{{Name: "invoke", Desc: "()V", Public: true, ExceptionsKnown: true}}}
			kind := InvokeVirtual
			receiverType := types.NewJavaClass("sample.Implementation")
			switch variant {
			case "interface declaration":
				declaration.IsInterface, kind = true, InvokeInterface
			case "same owner":
				receiverType = types.NewJavaClass("sample.Contract")
			case "unrelated receiver":
				implementation.Parents = nil
			case "inaccessible owner":
				declaration.Public = false
			case "generic declaration":
				declaration.Methods[0].Generic = true
			case "bridge declaration":
				declaration.Methods[0].Bridge = true
			case "wrong invoke kind":
				kind = InvokeInterface
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
				switch name {
				case declaration.Name:
					return declaration, variant != "missing original"
				case implementation.Name:
					return implementation, true
				}
				return callbinding.Class{}, false
			}}
			producer := &FunctionCallExpression{ClassName: "sample.Factory", FunctionName: "make", IsStatic: true, Kind: InvokeStatic, FuncType: &types.JavaFuncType{ReturnType: receiverType}}
			call := &FunctionCallExpression{Object: producer, ClassName: "sample.Contract", FunctionName: "invoke", Descriptor: "()V", Kind: kind, OriginPC: 11, HasOriginPC: true}
			planned, ok := call.planCallBinding(ctx)
			want := variant == "class declaration" || variant == "interface declaration"
			if ok != want {
				t.Fatalf("declaration view=%v, want=%v", ok, want)
			}
			if call.Object != producer {
				t.Fatal("planning mutated the original invocation")
			}
			if ok {
				cast, valid := planned.Object.(*CastExpression)
				if !valid || !cast.Binding || cast.Value != producer || bindingType(cast.TargetType) != "Lsample/Contract;" || planned.Witness() != call.Witness() {
					t.Fatal("declaration view lost producer identity or original invocation witness")
				}
			}
		})
	}
}
