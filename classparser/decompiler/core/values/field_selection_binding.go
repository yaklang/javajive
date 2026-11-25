package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/fieldbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A store target must never acquire a GETFIELD/boolean-value witness. Its
// separate immutable anchor only identifies the physical field selection.
func (f *RefMember) MarkOriginalFieldWrite(member *JavaClassMember, pc int) {
	if f != nil && f.originalFieldWrite == nil && f.originalFieldRead == nil && member != nil && f.Member == member.Member {
		f.originalFieldWrite = originalRead(member, pc)
		f.OriginPC, f.HasOriginPC = pc, true
	}
}
func (f *RefMember) sourceFieldReceiver(ctx *class_context.ClassContext) JavaValue {
	if f == nil || ctx == nil || ctx.OriginalFieldDeclarations == nil || ctx.InvocationMetadata == nil || isNilJavaValue(f.Object) || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || class_context.SafeIdentifier(f.Member) != f.Member {
		return nil
	}
	w := f.originalFieldRead
	if w == nil {
		w = f.originalFieldWrite
	}
	if w == nil || w.pc != f.OriginPC || w.name != f.Member {
		return nil
	}
	receiver, known := types.RawClassFQN(f.Object.Type())
	if !known {
		return nil
	}
	receiver = strings.ReplaceAll(receiver, ".", "/")
	owner := strings.ReplaceAll(w.owner, ".", "/")
	caller := strings.ReplaceAll(ctx.ClassName, ".", "/")
	if receiver == owner {
		return nil
	}
	widen := func(a, b string) bool { return callbinding.Assignable("L"+a+";", "L"+b+";", ctx.InvocationMetadata) }
	cast, known := fieldbinding.Selection(ctx.OriginalFieldDeclarations, widen, receiver, owner, w.name, w.descriptor, caller, ctx.Work)
	if !known || cast == "" || !strings.Contains(cast, "/") && ctx.IsTypeParam(cast) {
		return nil
	}
	return &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(strings.ReplaceAll(cast, "/", ".")), Binding: true}
}

// CanCastObjectReceiver reports whether widening this field receiver to java.lang.Object
// still renders a concrete owner cast ((Owner)(recv)).field via objectReceiverFieldCast.
func (f *RefMember) CanCastObjectReceiver(ctx *class_context.ClassContext) bool {
	if f == nil || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || class_context.SafeIdentifier(f.Member) != f.Member {
		return false
	}
	if isNilJavaValue(f.Object) || (ctx != nil && ctx.Getenv("JDEC_OBJECT_RECV_FIELD_CAST_OFF") != "") {
		return false
	}
	w := f.originalFieldRead
	if w == nil {
		w = f.originalFieldWrite
	}
	if w == nil || w.pc != f.OriginPC || w.name != f.Member || w.owner == "" {
		return false
	}
	ref, ok := UnpackSoltValue(f.Object).(*JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil {
		return false
	}
	owner := strings.ReplaceAll(w.owner, "/", ".")
	if owner == "java.lang.Object" || strings.HasPrefix(owner, "[") {
		return false
	}
	if ctx != nil && !strings.Contains(owner, ".") && ctx.IsTypeParam(owner) {
		return false
	}
	return true
}

// objectReceiverFieldCast returns the ShortTypeName cast target when a
// GETFIELD or PUTFIELD accesses a member on a plain local variable whose
// static type is java.lang.Object and whose decoded fieldref owner is a
// distinct concrete reference class (mirroring objectReceiverInvokeCast).
func (f *RefMember) objectReceiverFieldCast(ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || class_context.SafeIdentifier(f.Member) != f.Member {
		return ""
	}
	if isNilJavaValue(f.Object) || ctx.Getenv("JDEC_OBJECT_RECV_FIELD_CAST_OFF") != "" {
		return ""
	}
	w := f.originalFieldRead
	if w == nil {
		w = f.originalFieldWrite
	}
	if w == nil || w.pc != f.OriginPC || w.name != f.Member || w.owner == "" {
		return ""
	}
	ref, ok := UnpackSoltValue(f.Object).(*JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil {
		return ""
	}
	rt := ref.Type()
	if rt == nil {
		return ""
	}
	if jc, ok := rt.RawType().(*types.JavaClass); !ok || jc == nil || jc.Name != "java.lang.Object" {
		return ""
	}
	owner := strings.ReplaceAll(w.owner, "/", ".")
	if owner == "java.lang.Object" || strings.HasPrefix(owner, "[") {
		return ""
	}
	if !strings.Contains(owner, ".") && ctx.IsTypeParam(owner) {
		return ""
	}
	return ctx.ShortTypeName(owner)
}
