package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// A handle to an ordinary method does not consume a hidden enclosing
// constructor operand. Its original symbolic owner, name, physical descriptor,
// and invocation kind survive native member regeneration. Require a directly
// declared, unchanged target; inherited resolution, foreign private/special access,
// generated bridges/lambda bodies, fields, and enclosing constructors keep the existing
// refusal. Member interfaces use a separate declaration/tag/erasure proof below.
// Public access is unchanged. Package access additionally retains the original
// lookup package and exact owned source declaration (JVMS 5.4.4 / JLS 6.6.1).
// A separately proved ordinary static member has no enclosing operand
// in its public constructor either. This grants no private lookup privilege.
// A root's own private ordinary method uses the separate original-lookup
// certificate below; constructor and foreign observers retain their refusal.
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
		if !nativeProofWork(work, 1) {
			return false
		}
		if closedTargets[target] {
			continue
		}
		m := methods[declarationKey{target.name, target.descriptor}]
		if len(sourceMembers) == 1 && target.referencer == owner && sourceMembers[0] != nil && sourceMembers[0].object == obj && nativeMemberLambdaImplementation(sourceMembers[0], m, work) {
			if !nativeLambdaImplementationKind(obj, m, target.kind, !target.methodRef) {
				return false
			}
			closedTargets[target] = true
			continue
		}
		if len(sourceMembers) == 1 && nativeMemberOwnPrivateHandleSourceClosed(obj, sourceMembers[0], m, target, work) {
			closedTargets[target] = true
			continue
		}
		if target.kind == 9 {
			if target.methodRef || len(sourceMembers) != 1 || !nativeMemberInterfaceHandleSourceClosed(obj, sourceMembers[0], m, target, work) {
				return false
			}
			closedTargets[target] = true
			continue
		}
		if !target.methodRef {
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
		} else if target.kind != 5 && target.kind != 6 || target.name == "" || target.name == "<init>" || target.name == "<clinit>" || !class_context.PreservesJava8Identifier(target.name) {
			return false
		}
		if _, ret, err := callbinding.Descriptor(target.descriptor); err != nil || constructor && ret != "V" {
			return false
		}
		accessClosed := m != nil && m.AccessFlags&7 == 1
		if m != nil && !constructor && m.AccessFlags&7 == 0 && len(sourceMembers) == 1 {
			accessClosed = nativeMemberPackageHandleSourceClosed(obj, sourceMembers[0], target.referencer, work)
		}
		if m == nil || !accessClosed || m.AccessFlags&(0x0040|0x1000) != 0 || (m.AccessFlags&8 != 0) != (target.kind == 6) {
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

// Package access survives only in the unchanged original package and lexical
// declaration. This grants no private, protected, special or constructor access.
func nativeMemberPackageHandleSourceClosed(obj *ClassObject, member *nativeMemberClass, referencer string, work *workbudget.Budget) bool {
	if obj == nil || member == nil || member.object != obj || member.enumSynthesis != nil || referencer == "" || !nativeProofWork(work, int64(len(referencer))+int64(len(obj.GetClassName()))+1) || sourceBinaryPackage(referencer) != sourceBinaryPackage(obj.GetClassName()) {
		return false
	}
	for _, binary := range []string{referencer, obj.GetClassName()} {
		rest := binary
		for {
			head, tail, more := strings.Cut(rest, "/")
			if !class_context.PreservesJava8Identifier(head) {
				return false
			}
			if !more {
				break
			}
			rest = tail
		}
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
	return known && owner == member.owner && name == member.name && flags == member.flags && (flags&8 != 0) == member.static && flags&(0x0200|0x2000|0x4000) == 0 && nativeMemberDeclarationKindRepresentable(obj, flags, work)
}
