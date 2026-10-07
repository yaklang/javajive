package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Anonymous-super arguments are rendered in the caller's lexical scope. A
// decoded this reference has an erased JVM type, but its source view includes
// the original class formals. Preserve that view before wrapping the rendered
// operand; otherwise invariant constructor binding mistakes Owner<V> for raw
// Owner and omits an erased unchecked conversion. This changes no value or
// descriptor and grants nothing to an ordinary raw parameter or foreign this.
func nativeAnonymousArgumentSourceType(operand values.JavaValue, ctx *class_context.ClassContext, work *workbudget.Budget) types.JavaType {
	if sourceProofNil(operand) || operand.Type() == nil || !nativeProofWork(work, 1) {
		return nil
	}
	original := operand.Type().Copy()
	if ctx == nil || ctx.IsStatic || ctx.ClassSig == "" {
		return original
	}
	ref, known := values.UnpackSoltValue(operand).(*values.JavaRef)
	if !known || ref == nil || !ref.IsThis {
		return original
	}
	if _, parameterized := types.AsParameterizedType(original); parameterized {
		return original
	}
	raw, known := types.RawClassFQN(original)
	if !known || strings.ReplaceAll(raw, ".", "/") != strings.ReplaceAll(ctx.ClassName, ".", "/") {
		return original
	}
	formals := types.ClassFormalTypeParamNames(ctx.ClassSig)
	if len(formals) == 0 || len(formals) > 64 {
		return original
	}
	if _, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{ctx.ClassSig}, "()V"); !valid {
		return original
	}
	args := make([]types.JavaType, len(formals))
	for i, name := range formals {
		if !nativeProofWork(work, 1) {
			return nil
		}
		args[i] = types.NewJavaClass(name)
	}
	return types.NewParameterizedType(raw, args)
}
