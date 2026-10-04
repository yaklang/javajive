package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core"

// Regeneration is distinct from commuting a prefix store past delegation. The
// caller supplies a witness only after the exact source capture assignment was
// removed. Rebind it to the original complete family and exact bytecode prefix;
// a flat class, a stale plan or another pre-super effect cannot use this proof.
func (c *ClassObjectDumper) nativeMemberImplicitSuperRegenerated(method *MemberInfo, witness *nativeMemberConstructor, decoder *core.Decompiler) bool {
	child := c.nativeMemberCurrent
	if child == nil || child.static || witness == nil || decoder == nil || c.nativeMemberRoot == nil || c.nativeMemberRoot.children[c.obj.GetClassName()] != child || child.object != c.obj {
		return false
	}
	descriptor, ok := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !ok || child.constructors[descriptor] != witness || witness.descriptor != descriptor || witness.capturePC < 0 || witness.delegateOwner != c.obj.GetSupperClassName() || witness.delegateDescriptor != "()V" {
		return false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 5 || !nativeProofWork(c.Work, 5) {
		return false
	}
	field := constructorMotionMember(c.obj, ops[2], core.OP_PUTFIELD)
	if field == nil || field.Name != c.obj.GetClassName() || field.Member != child.field || field.Description != "L"+child.owner+";" || int(ops[2].CurrentOffset) != witness.capturePC || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[1]) != 1 || !constructorMotionLoad(ops[1], field.Description) || core.GetRetrieveIdx(ops[3]) != 0 || !constructorMotionLoad(ops[3], "Ljava/lang/Object;") {
		return false
	}
	delegate := constructorMotionMember(c.obj, ops[4], core.OP_INVOKESPECIAL)
	return delegate != nil && delegate.Name == witness.delegateOwner && delegate.Member == "<init>" && delegate.Description == "()V" && int(ops[4].CurrentOffset) == witness.delegatePC
}
