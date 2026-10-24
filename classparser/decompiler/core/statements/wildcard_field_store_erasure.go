package statements

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Equal erasures do not imply cast-convertible parameterizations. A captured
// factory result can be provably distinct from a nested invariant field type.
// Restore the actual PUTFIELD operand's erased view before the existing generic
// field view. All three erasures must equal the immutable original descriptor:
// this adds no runtime check, changes no producer/argument/overload, and renders
// the original operands only once. Lambdas, conditionals and unbound synthesized
// stores cannot borrow this direct invocation's receipt or acquire a new target.
func (a *AssignStatement) wildcardFieldStoreErasure(ctx *class_context.ClassContext, value values.JavaValue) string {
	if a == nil || ctx == nil || value == nil || originalStoreSeed(value) == nil || originalStoreSeed(value) != originalStoreSeed(a.JavaValue) {
		return ""
	}
	_, owner, name, descriptor, known := a.OriginalInstanceFieldStore()
	if !known || owner != strings.ReplaceAll(ctx.ClassName, ".", "/") {
		return ""
	}
	field, ok := a.LeftValue.(*values.RefMember)
	if !ok || field == nil {
		return ""
	}
	self, ok := values.UnpackSoltValue(field.Object).(*values.JavaRef)
	if !ok || self == nil || !self.IsThis || self.CustomValue != nil || self.StackVar != nil {
		return ""
	}
	signature := ctx.FieldSignature(name)
	if len(signature) == 0 || len(signature) > 4096 || ctx.Work != nil && (ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(signature))+1) != nil || ctx.Work.CheckAlloc(int64(len(signature))*8+128) != nil) {
		return ""
	}
	target := sameClassFieldGenericType(ctx, a.LeftValue)
	if target == nil || target.IsArray() {
		return ""
	}
	if _, parameterized := types.AsParameterizedType(target); !parameterized {
		return ""
	}
	targetErased, ok := values.SourceTypeErasure(target, ctx)
	if !ok || targetErased != descriptor {
		return ""
	}
	leftErased, ok := values.SourceTypeErasure(a.LeftValue.Type(), ctx)
	if !ok || leftErased != descriptor {
		return ""
	}
	call, ok := values.UnpackSoltValue(value).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC || call.OriginPC < 0 || call.OriginPC > 65535 || call.FuncType == nil || call.Kind != values.InvokeStatic && call.Kind != values.InvokeVirtual && call.Kind != values.InvokeInterface || call.IsStatic != (call.Kind == values.InvokeStatic) || call.IsSpecialInvoke || !call.IsStatic && call.Object == nil {
		return ""
	}
	parameters, result, err := callbinding.Descriptor(call.Descriptor)
	if err != nil || result != descriptor || len(parameters) != len(call.Arguments) || !strings.HasPrefix(descriptor, "L") {
		return ""
	}
	for _, argument := range call.Arguments {
		if argument == nil || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return ""
		}
	}
	resultErased, ok := values.SourceTypeErasure(call.FuncType.ReturnType, ctx)
	if !ok || resultErased != descriptor {
		return ""
	}
	raw, err := types.ParseDescriptor(descriptor)
	if err != nil {
		return ""
	}
	return raw.String(ctx)
}
