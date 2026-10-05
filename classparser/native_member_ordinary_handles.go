package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A handle to an ordinary public method does not consume a hidden enclosing
// constructor operand. Its original symbolic owner, name, physical descriptor,
// and invocation kind survive native member regeneration. Require a directly
// declared, unchanged target; inherited resolution, private/special access,
// generated bridges/lambda bodies, fields, and constructors keep the existing
// refusal. This certificate grants no lexical or private lookup privilege.
func nativeMemberOrdinaryHandlesClosed(obj *ClassObject, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if obj == nil || index == nil || !index.valid || !nativeProofWork(work, 1) {
		return false
	}
	owner := obj.GetClassName()
	if !index.handles[owner] {
		return true
	}
	targets := index.handleTargets[owner]
	if len(targets) == 0 || len(targets) > 4096 || len(obj.Methods) > 4096 || !nativeProofWork(work, int64(len(targets)+len(obj.Methods))) || work != nil && work.CheckAlloc(int64(len(targets)+len(obj.Methods))*96) != nil {
		return false
	}
	methods := map[string]*MemberInfo{}
	for _, m := range obj.Methods {
		if m == nil {
			return false
		}
		n, nok := sourceBridgeUTF8(obj, m.NameIndex)
		d, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		key := n + "\x00" + d
		if methods[key] != nil {
			return false
		}
		methods[key] = m
	}
	for _, target := range targets {
		if !nativeProofWork(work, 1) || !target.methodRef || target.kind != 5 && target.kind != 6 || target.name == "" || target.name == "<init>" || target.name == "<clinit>" || class_context.SafeIdentifier(target.name) != target.name {
			return false
		}
		if _, _, err := callbinding.Descriptor(target.descriptor); err != nil {
			return false
		}
		m := methods[target.name+"\x00"+target.descriptor]
		if m == nil || m.AccessFlags&7 != 1 || m.AccessFlags&(0x0040|0x1000) != 0 || (m.AccessFlags&8 != 0) != (target.kind == 6) {
			return false
		}
		for _, attr := range m.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			// Generic physical erasure needs its own unchanged source certificate.
			if _, generic := attr.(*SignatureAttribute); generic {
				return false
			}
		}
	}
	return true
}
