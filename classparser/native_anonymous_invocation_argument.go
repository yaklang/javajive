package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The source type of an anonymous creation comes from its original header,
// in its proved lexical scope. A numeric binary name or mutable rendered name
// cannot substitute for original ownership and exact allocation instructions.
// This query neither emits source nor changes any family/capture state.
func (c *ClassObjectDumper) nativeAnonymousArgumentParentSignature(owner, descriptor, sourceOwner, methodName, methodDesc string, newPC, pc int) (string, bool) {
	if c == nil || c.obj == nil || c.nativeAnonymousRoot == nil || !nativeProofWork(c.Work, 1) {
		return "", false
	}
	p := c.nativeAnonymousRoot
	child := p.children[strings.ReplaceAll(owner, ".", "/")]
	if p.failed || p.owner != c.obj.GetClassName() || p.owner != strings.ReplaceAll(sourceOwner, ".", "/") || child == nil || child.object == nil || child.object.GetClassName() != strings.ReplaceAll(owner, ".", "/") ||
		child.descriptor != descriptor || child.newPC != newPC || child.invokePC != pc || !nativeAnonymousAllocationScope(c.obj, child, methodName, methodDesc, c.Work) {
		return "", false
	}
	var original *MemberInfo
	var code *CodeAttribute
	if len(c.obj.Methods) > 4096 {
		return "", false
	}
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nk || !dk {
			return "", false
		}
		if name != methodName || desc != methodDesc {
			continue
		}
		if original != nil {
			return "", false
		}
		original = method
		for _, attribute := range method.Attributes {
			if body, ok := attribute.(*CodeAttribute); ok {
				if body == nil || code != nil {
					return "", false
				}
				code = body
			}
		}
	}
	bindings, known := c.nativeMemberAllocationInvocations(original, code)
	binding, found := bindings[newPC]
	if !known || !found || binding.pc != pc || binding.owner != child.object.GetClassName() || binding.descriptor != descriptor {
		return "", false
	}
	signature, valid := nativeMethodLocalOriginalSignature(child.object, child.object.Attributes, c.Work)
	if !valid || len(signature) > 8192 || len(child.object.Interfaces) > 1 || !nativeProofWork(c.Work, int64(len(signature)+1)) {
		return "", false
	}
	superName := child.object.GetSupperClassName()
	interfaceName := ""
	if len(child.object.Interfaces) == 1 {
		var known bool
		interfaceName, known = sourceBridgeClassName(child.object, child.object.Interfaces[0])
		if !known || superName != "java/lang/Object" {
			return "", false
		}
	}
	if signature == "" {
		if interfaceName != "" {
			return "Ljava/lang/Object;L" + interfaceName + ";", true
		}
		return "L" + superName + ";", superName != ""
	}
	depth := 0
	for _, ch := range signature {
		if ch == '<' {
			depth++
			if depth > 32 {
				return "", false
			}
		}
		if ch == '>' {
			depth--
			if depth < 0 {
				return "", false
			}
		}
	}
	if depth != 0 {
		return "", false
	}
	formals, _, known := types.SignatureTypeVariableReferences(signature)
	if !known || len(formals) != 0 {
		return "", false
	}
	super, interfaces := types.ParseClassSignatureSupers(signature)
	raw, known := types.RawClassFQN(super)
	if !known || strings.ReplaceAll(raw, ".", "/") != superName || len(interfaces) != len(child.object.Interfaces) {
		return "", false
	}
	if interfaceName != "" {
		raw, known = types.RawClassFQN(interfaces[0])
		if !known || strings.ReplaceAll(raw, ".", "/") != interfaceName {
			return "", false
		}
	}
	return signature, true
}
