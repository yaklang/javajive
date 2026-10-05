package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// REF_newInvokeSpecial to a public, ordinary static member constructor keeps
// its physical parameters unchanged. Bind to the accepted lexical declaration
// and original unique self row: a mutable static flag, an enclosing capture,
// or enum synthesis cannot license dropping a hidden operand. The caller checks
// each original constructor's flags, descriptor, bridges and generic Signature.
func nativeMemberStaticConstructorSourceClosed(obj *ClassObject, member *nativeMemberClass, work *workbudget.Budget) bool {
	if obj == nil || member == nil || member.object != obj || !member.static || member.field != "" || member.formalCount != 0 || member.enumSynthesis != nil {
		return false
	}
	for _, attribute := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if inner, ok := attribute.(*InnerClassesAttribute); ok && inner != nil && !nativeProofWork(work, int64(len(inner.Classes))) {
			return false
		}
	}
	owner, name, flags, known := originalMemberOwner(obj)
	return known && flags&8 != 0 && flags&(0x0200|0x4000) == 0 && owner == member.owner && name == member.name && flags == member.flags
}
