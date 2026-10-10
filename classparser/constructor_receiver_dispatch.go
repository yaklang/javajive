package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A final allocated receiver closes virtual dispatch, but does not select its
// superclass body by itself. Starting with the exact original receiver, prove
// its entire superclass path to the already resolved own declaration has no
// same-name/descriptor declaration. Any shadow (including bridges and private
// or static declarations) is conservatively refused. This new domain excludes
// interface/default and nonprivate/nonfinal special selection. JVMS 5.4.5/5.4.6:
// https://docs.oracle.com/javase/specs/jvms/se21/html/jvms-5.html#jvms-5.4.6
// This establishes only dispatch: storage, publication, finalization, returns,
// cycles and resource bounds still use the same receiver-effect proof.
func (c *ClassObjectDumper) constructorReceiverMethodDispatchClosed(obj *ClassObject, target *MemberInfo, member *values.JavaClassMember, opcode int, remaining *int) bool {
	if c == nil || obj == nil || target == nil || member == nil || remaining == nil || member.Name != obj.GetClassName() {
		return false
	}
	if target.AccessFlags&(0x0002|0x0010) != 0 {
		return true
	}
	if opcode != core.OP_INVOKEVIRTUAL || c.obj == nil || c.obj.AccessFlags&0x0010 == 0 || c.obj.AccessFlags&(0x0200|0x0400|0x8000) != 0 {
		return false
	}
	// Root method tables are caller-owned mutable observations. Record their
	// absence query in order with the original provider byte reads; revalidation
	// must repeat that query at this same point for every supported runtime.
	if evidence := c.constructorProfileEvidence; evidence != nil && evidence.inBody {
		if member.Name == c.obj.GetClassName() {
			evidence.eligible = false
		} else if !evidence.record(c, constructorProfileObservation{absentRootMethod: member.Member, methodDescriptor: member.Description}) {
			return false
		}
	}
	current, known := c.obj, true
	seen := map[string]bool{}
	for depth := 0; depth <= 16; depth++ {
		*remaining--
		name := current.GetClassName()
		if *remaining < 0 || name == "" || seen[name] || current.AccessFlags&(0x0200|0x8000) != 0 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(seen)+1)*64) != nil {
			return false
		}
		seen[name] = true
		if name == obj.GetClassName() {
			return current == obj
		}
		if !constructorReceiverRootMethodAbsent(current, member.Member, member.Description, remaining, c.Work) {
			return false
		}
		parent := current.GetSupperClassName()
		if parent == obj.GetClassName() {
			return depth < 16
		}
		current, known = c.constructorMotionClass(parent)
		if !known {
			return false
		}
	}
	return false
}

// Reject any matching declaration rather than approximate package/protected
// overriding or ignore private/static/bridge shadows. Different complete JVM
// descriptors remain separate methods. This same original query is replayed
// by the runtime certificate and shares the movement request's 512-unit cap.
func constructorReceiverRootMethodAbsent(object *ClassObject, name, descriptor string, remaining *int, work *workbudget.Budget) bool {
	if object == nil || remaining == nil || name == "" {
		return false
	}
	_, _, err := callbinding.Descriptor(descriptor)
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for _, method := range object.Methods {
		*remaining--
		if *remaining < 0 || method == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nameErr := object.getUtf8(method.NameIndex)
		d, descErr := object.getUtf8(method.DescriptorIndex)
		parameters, _, descriptorErr := callbinding.Descriptor(d)
		if nameErr != nil || descErr != nil || n == "" || descriptorErr != nil || !nativeProofWork(work, int64(len(d))) || n == name && d == descriptor {
			return false
		}
		width := nativeMemberParameterWidth(parameters)
		if method.AccessFlags&8 == 0 {
			width++
		}
		key := n + "\x00" + d
		if width > 255 || seen[key] || work != nil && work.CheckAlloc(int64(len(seen)+1)*64+int64(len(key))) != nil {
			return false
		}
		seen[key] = true
	}
	return true
}
