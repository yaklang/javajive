package javaclassparser

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
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
	parent, known := c.nativeAnnotationDeclarationResolver()(owner)
	if !known || parent == nil || parent.GetClassName() != owner || parent.AccessFlags&0x6201 != 0 || len(parent.Methods) > 4096 {
		return nil
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
			if target.AccessFlags != 1 || !known || !slices.Equal(throws, p.throws) {
				return false
			}
			matches++
		}
	}
	return matches == 1
}
