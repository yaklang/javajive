package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func (f *FunctionCallExpression) lexicalParameterizedReceiver(ctx *class_context.ClassContext) *types.JavaParameterizedType {
	if f == nil || f.Object == nil || ctx == nil {
		return nil
	}
	if _, field := UnpackSoltValue(f.Object).(*RefMember); !field {
		if pt, ok := types.AsParameterizedType(f.Object.Type()); ok && len(pt.OwnerSegments) > 1 {
			return pt
		}
	}
	if ctx.Getenv("JDEC_GENERIC_PARAM_FIELD_OFF") != "" {
		return nil
	}
	typ := RecoverThisFieldInstantiatedType(ctx, f.Object)
	if typ == nil {
		typ = recoverParameterizedFieldReceiver(ctx, f.Object)
	}
	pt, ok := types.AsParameterizedType(typ)
	if !ok || len(pt.OwnerSegments) < 2 {
		return nil
	}
	// A field Signature is class-scoped even inside a method declaring an equal
	// name. It cannot be rendered as that method's independent type variable.
	for _, segment := range pt.OwnerSegments {
		for _, arg := range segment.TypeArgs {
			for _, formal := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
				if raw, known := types.RawClassFQN(arg); known && raw == formal {
					return nil
				}
			}
		}
	}
	return pt
}
