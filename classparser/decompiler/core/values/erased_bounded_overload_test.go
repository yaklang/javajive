package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"testing"
)

func boundedOverloadFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class, *string) {
	f, ctx, meta, sig := methodInputFixture()
	f.Descriptor = "(Ljava/lang/Appendable;Ljava/util/Iterator;)Ljava/lang/Appendable;"
	*sig = "<A::Ljava/lang/Appendable;>(TA;Ljava/util/Iterator<*>;)TA;^Ljava/io/IOException;"
	for _, name := range []string{"java/lang/Appendable", "java/lang/StringBuilder", "java/util/Iterator", "java/io/IOException"} {
		meta[name] = callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true}
	}
	builder := meta["java/lang/StringBuilder"]
	builder.Parents = []string{"java/lang/Object", "java/lang/Appendable"}
	meta[builder.Name] = builder
	owner := meta["probe/Owner"]
	owner.Methods = []callbinding.Method{{Name: "apply", Desc: f.Descriptor, Generic: true, Public: true}, {Name: "apply", Desc: "(Ljava/lang/StringBuilder;Ljava/util/Iterator;)Ljava/lang/StringBuilder;", Public: true}}
	meta[owner.Name] = owner
	ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
		_, ok := meta[n]
		return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", f.Descriptor): *sig}, ok
	}
	f.IsStatic = false
	f.Kind = InvokeVirtual
	f.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("probe.Owner"))
	f.HasOriginPC = true
	f.OriginPC = 37
	ft, _ := types.ParseMethodDescriptor(f.Descriptor)
	f.FuncType = ft.FunctionType()
	f.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.StringBuilder")), NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.Iterator"))}
	return f, ctx, meta, sig
}

func TestErasedBoundedOverloadKeepsWitnessCheckedOperandsAndConsumer(t *testing.T) {
	f, ctx, _, _ := boundedOverloadFixture()
	operand := &CastExpression{Value: f.Arguments[0], TargetType: f.Arguments[0].Type(), OriginPC: 19}
	f.Arguments[0] = operand
	before := append([]JavaValue(nil), f.Arguments...)
	out, ok := f.PlanErasedDiscardedMethodInput(ctx)
	if !ok || out.Witness() != f.Witness() || out.Object != f.Object || out.Arguments[0].(*CastExpression).Value != operand || bindingType(out.Arguments[0].Type()) != "Ljava/lang/Appendable;" || !out.Arguments[0].(*CastExpression).Binding {
		t.Fatal("lost original bounded generic dispatch or check")
	}
	if !reflect.DeepEqual(before, f.Arguments) {
		t.Fatal("mutated shared call operands")
	}
	if _, ok := f.planErasedMethodInput(ctx); ok {
		t.Fatal("discarded-result permission escaped its consumer")
	}
}

func TestErasedBoundedOverloadRequiresCompleteFamilyAndSafeView(t *testing.T) {
	for _, variant := range []string{"original", "same erasure", "incomplete family", "different bound", "generic throws", "narrowing", "poly", "wrong staticness", "competing generic", "unknown signature"} {
		t.Run(variant, func(t *testing.T) {
			f, ctx, meta, sig := boundedOverloadFixture()
			owner := meta["probe/Owner"]
			switch variant {
			case "same erasure":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Appendable"))
			case "incomplete family":
				owner.MembersComplete = false
			case "different bound":
				*sig = "<A:Ljava/lang/String;>(TA;Ljava/util/Iterator<*>;)TA;"
			case "generic throws":
				*sig = "<A::Ljava/lang/Appendable;E:Ljava/lang/Exception;>(TA;Ljava/util/Iterator<*>;)TA;^TE;"
			case "narrowing":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "poly":
				f.Arguments[0] = &CustomValue{Flag: "lambda", TypeFunc: func() types.JavaType { return types.NewJavaClass("java.lang.StringBuilder") }}
			case "wrong staticness":
				owner.Methods[0].Static = true
			case "competing generic":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "apply", Desc: "(Ljava/lang/Object;Ljava/util/Iterator;)Ljava/lang/Object;", Generic: true, Public: true})
			case "unknown signature":
				ctx.SiblingClassSig = nil
			}
			meta[owner.Name] = owner
			if _, ok := f.PlanErasedDiscardedMethodInput(ctx); ok != (variant == "original") {
				t.Fatalf("bounded source view=%v", ok)
			}
		})
	}
}

func TestErasedClassScopedOverloadKeepsExistingReceiverBinding(t *testing.T) {
	f, ctx, meta, _ := boundedOverloadFixture()
	ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
		_, known := meta[n]
		return "<A::Ljava/lang/Appendable;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", f.Descriptor): "(TA;Ljava/util/Iterator<*>;)TA;^Ljava/io/IOException;"}, known
	}
	if _, ok := f.PlanErasedDiscardedMethodInput(ctx); ok {
		t.Fatal("class-scoped receiver inference borrowed method-formal overload permission")
	}
}
