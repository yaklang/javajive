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
// generated bridges/lambda bodies, fields, and enclosing constructors keep the existing
// refusal. A separately proved ordinary static member has no enclosing operand
// in its public constructor either. This grants no private lookup privilege.
func nativeMemberOrdinaryHandlesClosed(obj *ClassObject, index *nativeMemberIndex, work *workbudget.Budget, sourceMembers ...*nativeMemberClass) bool {
	if obj == nil || index == nil || !index.valid || len(sourceMembers) > 1 || !nativeProofWork(work, 1) {
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
	type declarationKey struct{ name, descriptor string }
	methods := map[declarationKey]*MemberInfo{}
	for _, m := range obj.Methods {
		if m == nil {
			return false
		}
		n, nok := sourceBridgeUTF8(obj, m.NameIndex)
		d, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		key := declarationKey{n, d}
		if methods[key] != nil {
			return false
		}
		methods[key] = m
	}
	// Original CPs may repeat the same target. Reuse only immutable physical
	// binding facts; each occurrence still pays for traversal/cancellation and
	// every distinct kind/tag/descriptor must close independently. Keys retain
	// existing metadata strings instead of allocating concatenated copies.
	closedTargets := map[nativeMemberHandleTarget]bool{}
	staticSourceChecked, staticSourceClosed := false, false
	for _, target := range targets {
		if !nativeProofWork(work, 1) || !target.methodRef {
			return false
		}
		if closedTargets[target] {
			continue
		}
		m := methods[declarationKey{target.name, target.descriptor}]
		if len(sourceMembers) == 1 && target.referencer == owner && sourceMembers[0] != nil && sourceMembers[0].object == obj && nativeMemberLambdaImplementation(sourceMembers[0], m, work) {
			if (m.AccessFlags&8 != 0 && target.kind == 6) || (m.AccessFlags&8 == 0 && (target.kind == 5 || target.kind == 7)) {
				closedTargets[target] = true
				continue
			}
			return false
		}
		constructor := target.kind == 8 && target.name == "<init>"
		if constructor {
			if !staticSourceChecked {
				staticSourceChecked = true
				staticSourceClosed = len(sourceMembers) == 1 && nativeMemberStaticConstructorSourceClosed(obj, sourceMembers[0], work)
			}
			if !staticSourceClosed || m == nil || m.AccessFlags & ^uint16(0x0081) != 0 || m.AccessFlags&1 == 0 || sourceMembers[0].accessBridges[target.descriptor] != nil {
				return false
			}
		} else if target.kind != 5 && target.kind != 6 || target.name == "" || target.name == "<init>" || target.name == "<clinit>" || class_context.SafeIdentifier(target.name) != target.name {
			return false
		}
		if _, ret, err := callbinding.Descriptor(target.descriptor); err != nil || constructor && ret != "V" {
			return false
		}
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
		closedTargets[target] = true
	}
	return true
}
