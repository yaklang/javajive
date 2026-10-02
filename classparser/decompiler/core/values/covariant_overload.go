package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"strings"
)

// A producer can have the exact descriptor result expected by its consumer,
// while Java resolves it on a narrower receiver and sees a covariant return.
// That return may implement two unrelated overload formals. Preserve the
// selected non-generic consumer formal with a proven widening result view.
// The producer stays evaluated once; its dynamic dispatch and checks stay put.
func (f *FunctionCallExpression) covariantOverloadResultCast(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || i < 0 || i >= len(f.Arguments) {
		return ""
	}
	child, ok := UnpackSoltValue(f.Arguments[i]).(*FunctionCallExpression)
	if !ok || child == nil || child.Object == nil || child.IsStatic || child.IsSpecialInvoke || child.Kind == InvokeDynamic {
		return ""
	}
	params, result, err := callbinding.Descriptor(child.Descriptor)
	target := f.witnessDescriptorParamType(i)
	if err != nil || len(params) != 0 || target == nil || !callbinding.Reference(result) || result != bindingType(target) {
		return ""
	}
	receiver := bindingType(child.Object.Type())
	owner := "L" + strings.ReplaceAll(child.ClassName, ".", "/") + ";"
	if receiver == owner || !strings.HasPrefix(receiver, "L") || !callbinding.Assignable(receiver, owner, ctx.InvocationMetadata) {
		return ""
	}
	kind := callbinding.Virtual
	if f.IsStatic {
		kind = callbinding.Static
	} else if f.Kind == InvokeInterface {
		kind = callbinding.Interface
	}
	consumer, e := callbinding.FamilyOf(callbinding.Witness{Owner: strings.ReplaceAll(f.ClassName, ".", "/"), Name: f.FunctionName, Desc: f.Descriptor, Kind: kind}, ctx.InvocationMetadata)
	if e != nil || consumer.Proof != callbinding.Compete || consumer.Target == nil || consumer.Target.Generic || consumer.Target.Varargs {
		return ""
	}
	source, known := ctx.InvocationMetadata(receiver[1 : len(receiver)-1])
	// A directly declared covariant override is positive return-type evidence;
	// unrelated missing ancestor method tables cannot erase that declaration.
	if !known || source.Name != receiver[1:len(receiver)-1] || !source.MembersComplete {
		return ""
	}
	for _, method := range source.Methods {
		if method.Name != child.FunctionName || !method.Public || method.Static || method.Generic || method.Bridge || method.Varargs {
			continue
		}
		ps, ret, e := callbinding.Descriptor(method.Desc)
		if e == nil && strings.Join(ps, "") == strings.Join(params, "") && ret != result && callbinding.Reference(ret) && callbinding.Assignable(ret, result, ctx.InvocationMetadata) {
			return target.String(ctx)
		}
	}
	return ""
}
