package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// ScopedErasureView supplies a source type-variable view only when its lexical
// first bound is exactly the operand's existing erasure. This does not add a
// narrower payload check. Method declarations shadow class declarations, and
// unresolved bounds fail closed; dependent bounds require complete declarations.
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
	// shadowing and proves dependent bounds without defaulting unknowns to Object.
	erased, known := SourceTypeErasure(target, ctx)
	return known && erased == bindingType(value.Type())
}

// SourceFieldType composes field/array access Signatures with the receiver's
// actual arguments. It never borrows unbound names from a raw generic owner.
func SourceFieldType(ctx *class_context.ClassContext, value JavaValue) types.JavaType {
	return recoverParameterizedFieldReceiver(ctx, value)
}

// ScopedFieldStoreView restores only an erased source binder at the original
// PUTFIELD selection. The receiver is evaluated once by the unchanged store;
// the added source cast has exactly the existing operand/field erasure.
func ScopedFieldStoreView(ctx *class_context.ClassContext, left, value JavaValue) types.JavaType {
	field, ok := UnpackSoltValue(left).(*RefMember)
	if !ok || field == nil || ctx == nil || field.originalFieldWrite == nil || !field.HasOriginPC || field.OriginPC < 0 || field.OriginPC > 65535 || field.Object == nil {
		return nil
	}
	if ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 2) != nil {
		return nil
	}
	w := field.originalFieldWrite
	receiver, known := types.RawClassFQN(field.Object.Type())
	if !known || !sameErasureClassName(receiver, w.owner) || w.pc != field.OriginPC || w.name != field.Member || w.descriptor != bindingType(field.Type()) {
		return nil
	}
	target := SourceFieldType(ctx, field)
	if target == nil {
		return nil
	}
	base := target
	for base.IsArray() {
		base = base.ElementType()
	}
	name, known := types.TypeVariableName(base)
	if !known || !ctx.IsTypeParam(name) || strings.Contains(name, ".") {
		return nil
	}
	for _, shadow := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		if shadow == name {
			return nil
		}
	}
	if !ScopedErasureView(ctx, target, value) {
		return nil
	}
	return target
}
