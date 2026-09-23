package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

func bindingType(t types.JavaType) string {
	if t == nil {
		return ""
	}
	if t.IsArray() {
		return strings.Repeat("[", t.ArrayDim()) + bindingType(t.ElementType())
	}
	if n := witnessRawClassName(t); n != "" {
		return "L" + strings.ReplaceAll(n, ".", "/") + ";"
	}
	return map[string]string{"byte": "B", "char": "C", "double": "D", "float": "F", "int": "I", "long": "J", "short": "S", "boolean": "Z", "void": "V"}[t.String(&dummyTypeCtx)]
}

// planCallBinding runs before text rendering. A widening cast changes javac's
// overload lookup while preserving the receiver object and dynamic dispatch.
func (f *FunctionCallExpression) planCallBinding(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if ctx == nil || ctx.InvocationMetadata == nil || f.Descriptor == "" || f.FunctionName == "<init>" || f.IsSpecialInvoke || f.Kind == InvokeSpecial || f.Kind == InvokeDynamic {
		return nil, false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	if _, ok := ctx.InvocationMetadata(owner); !ok {
		return nil, false
	}
	kind := callbinding.Virtual
	if f.IsStatic || f.Kind == InvokeStatic {
		kind = callbinding.Static
	} else if f.Kind == InvokeInterface {
		kind = callbinding.Interface
	}
	ps, _, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != len(f.Arguments) {
		return nil, false
	}
	args := make([]callbinding.Argument, len(f.Arguments))
	risk := false
	recv := ""
	if f.Object != nil {
		recv = bindingType(f.Object.Type())
		if _, ok := UnpackSoltValue(f.Object).(*JavaClassValue); ok && kind != callbinding.Static {
			recv = "Ljava/lang/Class;"
		}
		if kind != callbinding.Static && recv != "L"+owner+";" {
			risk = true
		}
	}
	for i, a := range f.Arguments {
		if a == nil {
			return nil, false
		}
		at := bindingType(a.Type())
		if _, ok := UnpackSoltValue(a).(*JavaClassValue); ok {
			at = "Ljava/lang/Class;"
		}
		if lit, ok := UnpackSoltValue(a).(*JavaLiteral); ok && lit.Data == nil {
			at = "null"
		}
		poly := false
		if cv, ok := UnpackSoltValue(a).(*CustomValue); ok {
			poly = cv.Flag == "lambda"
		}
		args[i] = callbinding.Argument{Type: at, Poly: poly}
		if at != ps[i] && callbinding.Reference(ps[i]) {
			risk = true
		}
	}
	if !risk {
		return nil, false
	}
	// With an exact raw receiver type, a complete Unique family, and arguments
	// already assignable to the erased descriptor, javac has no competing
	// overload to steal the call. In that case a widening cast is unnecessary;
	// requiring the planner to prove a generic/access/bridge rewrite would mark
	// valid raw calls such as List.add(String) unsupported for no source-level
	// benefit. Parameterized receivers and incomplete/competing families still
	// go through the conservative planner below.
	if f.rawReceiverHasUniqueErasedBinding(ctx, owner, ps, args) {
		return nil, false
	}
	plan := callbinding.Build(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor, Kind: kind, PC: f.OriginPC}, recv, args, ctx.InvocationMetadata)
	if !plan.Supported {
		// Existing generic target-typing paths own these calls. They must not be
		// replaced with erased Object casts by this bounded non-generic planner.
		if strings.Contains(plan.Reason, "generic/access/varargs/bridge") {
			return nil, false
		}
		ctx.OverloadFamilyUnproven = true
		if ctx.OnOverloadUnknown != nil {
			ctx.OnOverloadUnknown(f.ClassName, f.FunctionName, f.Descriptor)
		}
		return nil, false
	}
	out := f.Clone()
	if kind != callbinding.Static && recv != plan.ReceiverType {
		t, _ := types.ParseDescriptor(plan.ReceiverType)
		out.Object = &CastExpression{Value: f.Object, TargetType: t, OriginPC: f.OriginPC, Binding: true}
	}
	for i, desc := range plan.ArgumentTypes {
		if args[i].Type == desc {
			continue
		}
		t, e := types.ParseDescriptor(desc)
		if e != nil {
			return nil, false
		}
		out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: t, OriginPC: f.OriginPC, Binding: true}
	}
	out.bindingPlanned = true
	return out, true
}

func (f *FunctionCallExpression) rawReceiverHasUniqueErasedBinding(ctx *class_context.ClassContext, owner string, params []string, args []callbinding.Argument) bool {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || f.Object == nil ||
		f.IsStatic || f.Kind == InvokeStatic || f.IsSpecialInvoke || f.Kind == InvokeSpecial ||
		len(params) != len(args) {
		return false
	}
	receiverType := f.Object.Type()
	if receiverType == nil {
		return false
	}
	receiver, ok := receiverType.RawType().(*types.JavaClass)
	if !ok {
		return false
	}
	receiverOwner := strings.ReplaceAll(receiver.Name, ".", "/")
	metadata, ok := ctx.InvocationMetadata(receiverOwner)
	if !ok || !metadata.Public || !callbinding.Assignable("L"+receiverOwner+";", "L"+owner+";", ctx.InvocationMetadata) {
		return false
	}
	kind := callbinding.Virtual
	if metadata.IsInterface {
		kind = callbinding.Interface
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{
		Owner: receiverOwner, Name: f.FunctionName, Desc: f.Descriptor, Kind: kind,
	}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique {
		return false
	}
	for i, arg := range args {
		if !callbinding.Assignable(arg.Type, params[i], ctx.InvocationMetadata) {
			return false
		}
	}
	return true
}
