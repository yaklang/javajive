package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// A generic field's source type can be narrower than its original GETFIELD
// descriptor. Pin a competing non-generic invocation to that descriptor with
// a widening view. The field read, receiver, null check and evaluation count
// are unchanged. Declaring bounds must erase correctly BEFORE instantiation.
func (f *FunctionCallExpression) instantiatedFieldOverloadCast(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || ctx.SiblingFieldSig == nil || ctx.Getenv("JDEC_GENERIC_FIELD_CHAIN_OFF") != "" || ctx.Getenv("JDEC_INHERITED_FIELD_SIG_OFF") != "" ||
		f.FunctionName == "<init>" || f.IsSpecialInvoke || f.Kind == InvokeSpecial || f.Kind == InvokeDynamic || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		i < 0 || i >= len(f.Arguments) {
		return ""
	}
	field, ok := UnpackSoltValue(f.Arguments[i]).(*RefMember)
	if !ok || field == nil || field.originalFieldRead == nil || isNilJavaValue(field.Object) || field.Object.Type() == nil {
		return ""
	}
	w := field.originalFieldRead
	if !field.OriginalInstanceFieldRead(field.OriginPC, w.owner, w.name, w.descriptor) || bindingType(field.Type()) != w.descriptor || class_context.SafeIdentifier(field.Member) != field.Member {
		return ""
	}
	ps, _, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != len(f.Arguments) || ps[i] != w.descriptor || !callbinding.Reference(ps[i]) {
		return ""
	}
	raw, known := types.RawClassFQN(field.Object.Type())
	var args []types.JavaType
	if ref, own := UnpackSoltValue(field.Object).(*JavaRef); own && ref != nil && ref.IsThis {
		if ref.CustomValue != nil || ref.StackVar != nil || !sameOriginalFieldOwner(raw, ctx.ClassName) {
			return ""
		}
		raw = ctx.ClassName
		signature, _, available := ctx.SiblingClassSig(strings.ReplaceAll(raw, ".", "/"))
		if !available || signature != ctx.ClassSig {
			return ""
		}
		for _, formal := range types.ClassFormalTypeParamNames(ctx.ClassSig) {
			args = append(args, types.NewJavaClass(formal))
		}
	} else if receiver, parameterized := types.AsParameterizedType(recoverParameterizedFieldReceiver(ctx, field.Object)); parameterized {
		raw, args, known = receiver.RawClassName, receiver.TypeArgs, true
	}
	if !known || !sameOriginalFieldOwner(raw, w.owner) {
		return ""
	}
	source := types.ResolveInstantiatedFieldTypeWithErasure(ctx, ctx.SiblingClassSig, ctx.SiblingFieldSig, raw, args, field.Member, w.descriptor)
	actual, known := SourceTypeErasure(source, ctx)
	if source == nil || !known || !sourceDenotableJavaType(source, ctx) || !callbinding.Assignable(actual, ps[i], ctx.InvocationMetadata) {
		return ""
	}
	views, known := invocationSourceFormalViews(source, ctx)
	if !known {
		return ""
	}
	return f.narrowSourceOverloadCast(i, actual, ctx, views...)
}
