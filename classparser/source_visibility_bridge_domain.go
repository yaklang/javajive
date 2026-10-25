package javaclassparser

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// javac versions disagree about visibility forwarders for generic methods
// inherited from a non-public parent. Source inheritance must not invent an
// original child declaration. Restrict the domain to a public class and its
// exact non-public direct superclass, resolved from original declarations.
func (c *ClassObjectDumper) inheritedVisibilityDomainParent() *ClassObject {
	if c == nil || c.obj == nil || c.obj.MajorVersion < 49 || c.obj.MajorVersion > 52 || c.obj.MinorVersion != 0 || c.obj.AccessFlags&0x6201 != 1 || len(c.obj.Methods) > 4096 || !nativeProofWork(c.Work, 1) {
		return nil
	}
	owner := c.obj.GetSupperClassName()
	resolve := c.nativeAnnotationDeclarationResolver()
	parent, known := resolve(owner)
	if !known || parent == nil || parent.GetClassName() != owner || parent.AccessFlags&0x6200 != 0 || len(parent.Methods) > 4096 {
		return nil
	}
	if parent.AccessFlags&1 != 0 {
		// ClassFile access is not source access: a protected member class
		// has ACC_PUBLIC in its physical header. Its original reciprocal
		// InnerClasses edge carries the narrower lexical declaration. A
		// public top-level parent (or an unproved/mismatched edge) remains
		// outside this domain. Neither a fresh compiler row nor a '$' name
		// is evidence of that original declaration.
		if !nativeMemberDeclarationMetadataBounded(parent, c.Work) {
			return nil
		}
		outerName, _, flags, named := originalMemberOwner(parent)
		if !named || flags&7 != 4 || flags & ^uint16(0x761f) != 0 || parent.AccessFlags & ^uint16(0x7631) != 0 || flags&0x7610 != parent.AccessFlags&0x7610 {
			return nil
		}
		outer, known := resolve(outerName)
		if !known || outer == nil || outer.GetClassName() != outerName || !nativeOriginalDeclarationEdge(outer, parent, c.Work) {
			return nil
		}
	}
	return parent
}

// A removable fresh declaration is precisely an unchanged-descriptor, typed
// forwarding packet to the original public parent implementation. No casts,
// allocation, branch, handler or changed invocation kind is admitted. Removing
// this compiler-only declaration restores original JVM inheritance/lookup;
// it does not erase a user method or import any original executable bytes.
func (c *ClassObjectDumper) extraInheritedVisibilityBridge(fresh *ClassObject, method *MemberInfo, parent *ClassObject) bool {
	if c == nil || c.obj == nil || fresh == nil || parent == nil || parent.GetClassName() != c.obj.GetSupperClassName() || method == nil || method.AccessFlags != 0x1041 {
		return false
	}
	p := inheritedBridgeForwardPacket(fresh, method, core.OP_INVOKESPECIAL, c.Work)
	if p == nil || p.checkedArguments || p.targetOwner != parent.GetClassName() || p.targetDescriptor != p.descriptor {
		return false
	}
	for _, original := range c.obj.Methods {
		if original == nil {
			return false
		}
		name, nk := sourceBridgeUTF8(c.obj, original.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, original.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(c.Work, 1) || name == p.name && desc == p.descriptor {
			return false
		}
	}
	matches := 0
	for _, target := range parent.Methods {
		if target == nil {
			return false
		}
		name, nk := sourceBridgeUTF8(parent, target.NameIndex)
		desc, dk := sourceBridgeUTF8(parent, target.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(c.Work, 1) {
			return false
		}
		if name == p.name && desc == p.descriptor {
			throws, known := originalMethodExceptions(parent, target)
			if target.AccessFlags&^uint16(0x80) != 1 || !known || !slices.Equal(throws, p.throws) {
				return false
			}
			// A resolved JVM invocation passes its array port unchanged. Varargs
			// is a source-call convention, not an executable adaptation. Admit
			// that marker only when its physical final parameter really is an
			// array; the pure forwarding proof above still forbids allocation,
			// casts, repacking or a different target descriptor.
			if target.AccessFlags&0x80 != 0 {
				params, _, err := callbinding.Descriptor(desc)
				if err != nil || len(params) == 0 || !strings.HasPrefix(params[len(params)-1], "[") {
					return false
				}
			}
			matches++
		}
	}
	return matches == 1
}
