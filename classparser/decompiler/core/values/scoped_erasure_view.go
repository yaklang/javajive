package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// ScopedErasureView supplies a source type-variable view only when its lexical
// first bound is exactly the operand's existing erasure. This does not add a
// narrower payload check. Method declarations shadow class declarations, and
// unresolved/dependent bounds fail closed.
func ScopedErasureView(ctx *class_context.ClassContext, target types.JavaType, value JavaValue) bool {
	if ctx == nil || target == nil || value == nil || value.Type() == nil || ctx.Getenv("JDEC_SCOPED_ERASURE_VIEW_OFF") != "" {
		return false
	}
	base := target
	for base.IsArray() {
		base = base.ElementType()
	}
	name, known := types.RawClassFQN(base)
	if !known || !ctx.IsTypeParam(name) || ctx.RawEraseTypeVar(name) || bindingType(target) == bindingType(value.Type()) || IsNullLiteral(UnpackSoltValue(value)) {
		return false
	}
	// SourceTypeErasure walks original lexical declarations nearest-first,
	// including enclosing class formals of a regenerated member. It retains
	// shadowing and rejects unknown/dependent bounds rather than using Object.
	erased, known := SourceTypeErasure(target, ctx)
	return known && erased == bindingType(value.Type())
}

// SourceFieldType composes field/array access Signatures with the receiver's
// actual arguments. It never borrows unbound names from a raw generic owner.
func SourceFieldType(ctx *class_context.ClassContext, value JavaValue) types.JavaType {
	return recoverParameterizedFieldReceiver(ctx, value)
}
