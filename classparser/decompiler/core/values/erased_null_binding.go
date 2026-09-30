package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The bytecode chose an Object-parameter instance method. Replacing its null
// argument with (Object)null on a captured receiver is rejected by javac;
// emitting bare null can select a String overload instead. When the receiver's
// parameterized erasure is exactly the invoke owner, a raw receiver plus the
// Object argument cast preserves that descriptor and virtual dispatch. The
// receiver is evaluated once and the erasure cast adds no runtime type check.
// Void results avoid changing generic return inference at the use site. No
// dependency declaration or guess about the owner's type-variable names is
// needed, which matters when decompiling an isolated classfile.
func (f *FunctionCallExpression) planErasedNullBinding(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || f.Object == nil || f.IsStatic || f.IsSpecialInvoke ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || f.FunctionName == "<init>" ||
		f.Descriptor != "(Ljava/lang/Object;)V" || len(f.Arguments) != 1 {
		return nil, false
	}
	arg, ok := UnpackSoltValue(f.Arguments[0]).(*JavaLiteral)
	if !ok || arg == nil || !IsNullLiteral(arg) || bindingType(arg.Type()) != "Ljava/lang/Object;" {
		return nil, false
	}
	raw, args := f.receiverParamTypeArgs(ctx)
	if raw == "" || len(args) == 0 || !sameErasureClassName(raw, f.ClassName) {
		return nil, false
	}
	out := f.Clone()
	out.Object = &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(raw), OriginPC: f.OriginPC, Binding: true}
	out.Arguments[0] = &CastExpression{Value: f.Arguments[0], TargetType: types.NewJavaClass("java.lang.Object"), OriginPC: f.OriginPC, Binding: true}
	out.bindingPlanned = true
	return out, true
}
