package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// recoverParameterizedFieldReceiver composes field Signatures through an
// explicitly parameterized receiver: holder<T>.nested<U>.callback. GETFIELD
// records only erasures, so the ordinary Type() of either field cannot carry
// this evidence. Resolve each edge independently and require the recovered
// field's erasure to agree with the bytecode. Raw receivers never borrow the
// declaring class's unbound type-variable names, even if a caller has an
// unrelated type variable with the same spelling.
func recoverParameterizedFieldReceiver(ctx *class_context.ClassContext, value JavaValue) types.JavaType {
	if ctx == nil || ctx.Getenv("JDEC_GENERIC_FIELD_CHAIN_OFF") != "" {
		return nil
	}
	seen := map[*RefMember]bool{}
	var recover func(JavaValue, int) types.JavaType
	recover = func(value JavaValue, depth int) types.JavaType {
		if value == nil || depth > 64 {
			return nil
		}
		if _, ok := types.AsParameterizedType(value.Type()); ok {
			return value.Type()
		}
		field, ok := UnpackSoltValue(value).(*RefMember)
		if !ok || field == nil || seen[field] {
			return nil
		}
		seen[field] = true
		defer delete(seen, field)
		if own := RecoverThisFieldInstantiatedType(ctx, field); own != nil {
			return own
		}
		if ctx.SiblingClassSig == nil || ctx.SiblingFieldSig == nil {
			return nil
		}
		receiver, ok := types.AsParameterizedType(recover(field.Object, depth+1))
		if !ok || receiver == nil {
			return nil
		}
		classSig, _, known := ctx.SiblingClassSig(strings.ReplaceAll(receiver.RawClassName, ".", "/"))
		if !known || len(types.ClassFormalTypeParamNames(classSig)) != len(receiver.TypeArgs) {
			return nil
		}
		recovered := types.ResolveInstantiatedFieldType(ctx, ctx.SiblingClassSig, ctx.SiblingFieldSig,
			receiver.RawClassName, receiver.TypeArgs, field.Member)
		declaredRaw, declaredOK := types.RawClassFQN(field.Type())
		recoveredRaw, recoveredOK := types.RawClassFQN(recovered)
		if !declaredOK || !recoveredOK || !sameErasureClassName(declaredRaw, recoveredRaw) || !sourceDenotableJavaType(recovered, ctx) {
			return nil
		}
		return recovered
	}
	return recover(value, 0)
}
