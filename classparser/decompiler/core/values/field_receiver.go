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
		if array, ok := UnpackSoltValue(value).(*JavaArrayMember); ok && array != nil {
			if source := recover(array.Object, depth+1); source != nil && source.IsArray() {
				return source.ElementType()
			}
			return nil
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
		if own := recoverThisTypeVariableField(ctx, field); own != nil {
			return own
		}
		if lexical := recoverLexicalEnclosingFieldReceiver(ctx, field); lexical != nil {
			return lexical
		}
		if ctx.SiblingClassSig == nil || ctx.SiblingFieldSig == nil {
			return nil
		}
		receiver, ok := types.AsParameterizedType(recover(field.Object, depth+1))
		if !ok || receiver == nil {
			raw, known := types.RawClassFQN(field.Object.Type())
			sig, _, available := ctx.SiblingClassSig(strings.ReplaceAll(raw, ".", "/"))
			if !known || !available || len(types.ClassFormalTypeParamNames(sig)) != 0 {
				return nil
			}
			receiver, _ = types.AsParameterizedType(types.NewParameterizedType(raw, nil))
		}
		classSig, _, known := ctx.SiblingClassSig(strings.ReplaceAll(receiver.RawClassName, ".", "/"))
		if !known || len(types.ClassFormalTypeParamNames(classSig)) != len(receiver.TypeArgs) {
			return nil
		}
		recovered := types.ResolveInstantiatedFieldType(ctx, ctx.SiblingClassSig, ctx.SiblingFieldSig,
			receiver.RawClassName, receiver.TypeArgs, field.Member)
		declaredRaw, declaredOK := types.RawClassFQN(field.Type())
		recoveredRaw, recoveredOK := types.RawClassFQN(recovered)
		matches := declaredOK && recoveredOK && sameErasureClassName(declaredRaw, recoveredRaw)
		if !matches && recovered != nil {
			matches = ScopedErasureView(ctx, recovered, field)
		}
		if !matches || !sourceDenotableJavaType(recovered, ctx) {
			return nil
		}
		return recovered
	}
	return recover(value, 0)
}

// Bare/array field formals are recorded separately from parameterized object
// Signatures. In a member their declaration may belong to an enclosing class;
// an empty member ClassSig cannot supply or erase that binder by itself.
func recoverThisTypeVariableField(ctx *class_context.ClassContext, field *RefMember) types.JavaType {
	ref, known := UnpackSoltValue(field.Object).(*JavaRef)
	if !known || ref == nil || !ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
		return nil
	}
	decl := ctx.FieldTypeVar(class_context.SafeIdentifier(field.Member))
	name := strings.TrimSuffix(decl, strings.Repeat("[]", strings.Count(decl, "[]")))
	if name == "" || !ctx.IsTypeParam(name) {
		return nil
	}
	for _, shadow := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		if name == shadow {
			return nil
		}
	}
	target := types.ParseSignature(strings.Repeat("[", strings.Count(decl, "[]")) + "T" + name + ";")
	erased, valid := SourceTypeErasure(target, ctx)
	if !valid || erased != bindingType(field.Type()) {
		return nil
	}
	return target
}
