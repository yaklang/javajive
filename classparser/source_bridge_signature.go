package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// javac accessibility bridges expose a public method inherited from a hidden
// superclass. The dumper omits bridges, so source lookup sees that superclass's
// generic declaration. Original JVM metadata remains untouched. Only an exact
// category-preserving, same-descriptor invokespecial forwarder supplies this separate
// source-view evidence; arbitrary bridges or rewritten bodies cannot supply it.
func (c *ClassObjectDumper) buildSourceBridgeTargets() func(string, string, string) (string, bool) {
	cache := map[string]map[string]string{}
	originals := map[string]*ClassObject{}
	retained := 0
	resolve := func(name string) (*ClassObject, bool) {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if original, seen := originals[name]; seen {
			return original, original != nil
		}
		if c.obj != nil {
			n, known := sourceBridgeClassName(c.obj, c.obj.ThisClass)
			if !known {
				return nil, false
			}
			if n == name {
				return c.obj, true
			}
		}
		if len(originals) >= 256 {
			return nil, false
		}
		originals[name] = nil
		var raw []byte
		known := false
		if c.foldSiblingResolver != nil {
			raw, known = c.foldSiblingResolver(name)
		}
		if !known && c.archiveDeclarationResolver != nil {
			raw, known = c.archiveDeclarationResolver(name)
		}
		if !known && c.declarationResolver != nil {
			raw, known = c.declarationResolver(name)
		}
		if !known || len(raw) > 2<<20 || retained+len(raw) > 16<<20 || c.Work != nil && (c.Work.Charge(workbudget.CounterReadBytes, int64(len(raw))) != nil || c.Work.CheckAlloc(int64(retained+len(raw))*128+int64(len(originals))*128) != nil) {
			return nil, false
		}
		retained += len(raw)
		obj, err := c.parseResolved(bytes.Clone(raw))
		if err != nil || obj == nil {
			return nil, false
		}
		n, valid := sourceBridgeClassName(obj, obj.ThisClass)
		if !valid || n != name {
			return nil, false
		}
		originals[name] = obj
		return obj, true
	}
	return func(owner, name, descriptor string) (string, bool) {
		if c == nil || c.obj == nil || len(owner) > 4096 || len(name) > 4096 || len(descriptor) > 4096 || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		methods, seen := cache[owner]
		if !seen {
			if len(cache) >= 256 || c.Work != nil && c.Work.CheckAlloc(int64(len(cache)+1)*128) != nil {
				return "", false
			}
			methods = map[string]string{}
			cache[owner] = methods
			if obj, ok := resolve(owner); ok && len(obj.Methods) <= 4096 && nativeProofWork(c.Work, int64(len(obj.Methods))+1) && (c.Work == nil || c.Work.CheckAlloc(int64(len(obj.Methods))*128) == nil) {
				counts := map[string]int{}
				for _, method := range obj.Methods {
					if method == nil || !nativeProofWork(c.Work, 1) {
						methods = map[string]string{}
						break
					}
					n, nameOK := sourceBridgeUTF8(obj, method.NameIndex)
					d, descriptorOK := sourceBridgeUTF8(obj, method.DescriptorIndex)
					if !nameOK || !descriptorOK || len(n) > 4096 || len(d) > 4096 || !nativeProofWork(c.Work, int64(len(n)+len(d))+1) {
						methods = map[string]string{}
						break
					}
					key := class_context.MethodDescKey(n, d)
					counts[key]++
					if target, proved := c.sourceBridgeForwardTarget(obj, method, resolve); proved {
						methods[key] = target
					}
				}
				for _, count := range counts {
					if count != 1 {
						methods = map[string]string{}
						break
					}
				}
			}
			cache[owner] = methods // Never retain a partially scanned table after refusal.
		}
		target, ok := methods[class_context.MethodDescKey(name, descriptor)]
		return target, ok
	}
}

func (c *ClassObjectDumper) sourceBridgeForwardTarget(obj *ClassObject, method *MemberInfo, resolve func(string) (*ClassObject, bool)) (string, bool) {
	if obj == nil || method == nil || method.AccessFlags != 0x1041 || len(method.Attributes) > 128 || obj.AccessFlags&0x201 != 1 || !nativeProofWork(c.Work, 1) {
		return "", false
	}
	name, nameOK := sourceBridgeUTF8(obj, method.NameIndex)
	desc, descriptorOK := sourceBridgeUTF8(obj, method.DescriptorIndex)
	if !nameOK || !descriptorOK || name == "" || strings.HasPrefix(name, "<") {
		return "", false
	}
	params, result, err := callbinding.Descriptor(desc)
	if err != nil || len(params) > 8 {
		return "", false
	}
	loads := []byte{core.OP_ALOAD_0}
	slot := 1
	for _, param := range params {
		var base, compact byte
		words := 1
		switch param[0] {
		case 'L', '[':
			base, compact = core.OP_ALOAD, core.OP_ALOAD_0
		case 'J':
			base, compact = core.OP_LLOAD, core.OP_LLOAD_0
			words = 2
		case 'D':
			base, compact = core.OP_DLOAD, core.OP_DLOAD_0
			words = 2
		case 'F':
			base, compact = core.OP_FLOAD, core.OP_FLOAD_0
		default:
			base, compact = core.OP_ILOAD, core.OP_ILOAD_0
		}
		if slot <= 3 {
			loads = append(loads, compact+byte(slot))
		} else {
			loads = append(loads, base, byte(slot))
		}
		slot += words
	}
	ret := byte(core.OP_IRETURN)
	returnWords := 1
	switch result[0] {
	case 'L', '[':
		ret = core.OP_ARETURN
	case 'J':
		ret = core.OP_LRETURN
		returnWords = 2
	case 'D':
		ret = core.OP_DRETURN
		returnWords = 2
	case 'F':
		ret = core.OP_FRETURN
	case 'V':
		ret = core.OP_RETURN
		returnWords = 0
	}
	stack := slot
	if returnWords > stack {
		stack = returnWords
	}
	var code *CodeAttribute
	seenParameters := false
	for _, attribute := range method.Attributes {
		if sourceProofNil(attribute) || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		switch attr := attribute.(type) {
		case *SignatureAttribute:
			return "", false // An explicit declaration remains authoritative.
		case *CodeAttribute:
			if code != nil {
				return "", false
			}
			code = attr
		case *UnparsedAttribute:
			if attr.Name != "MethodParameters" || seenParameters || len(attr.Info) != 1+4*len(params) || attr.Length != uint32(len(attr.Info)) || int(attr.Info[0]) != len(params) {
				return "", false
			}
			seenParameters = true
			for i := range params {
				entry := attr.Info[1+4*i : 1+4*(i+1)]
				if binary.BigEndian.Uint16(entry[:2]) != 0 || binary.BigEndian.Uint16(entry[2:]) != 0x1000 {
					return "", false
				}
			}
		default:
			return "", false
		}
	}
	if code == nil || int(code.MaxLocals) != slot || int(code.MaxStack) != stack || len(code.ExceptionTable) != 0 || len(code.Code) != len(loads)+4 || !bytes.Equal(code.Code[:len(loads)], loads) || code.Code[len(loads)] != core.OP_INVOKESPECIAL || code.Code[len(code.Code)-1] != ret {
		return "", false
	}
	cp := NewConstantPoolWithConstant(&obj.ConstantPool)
	ref, ok := cp.IndexInfo(int(binary.BigEndian.Uint16(code.Code[len(loads)+1 : len(loads)+3]))).(*ConstantMethodrefInfo)
	if !ok || ref == nil {
		return "", false
	}
	nt, ok := cp.IndexInfo(int(ref.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return "", false
	}
	n, nameOK := sourceBridgeUTF8(obj, nt.NameIndex)
	d, descriptorOK := sourceBridgeUTF8(obj, nt.DescriptorIndex)
	parent, targetOK := sourceBridgeClassName(obj, ref.ClassIndex)
	super, superOK := sourceBridgeClassName(obj, obj.SuperClass)
	owner, ownerOK := sourceBridgeClassName(obj, obj.ThisClass)
	if !nameOK || !descriptorOK || !targetOK || !superOK || !ownerOK || n != name || d != desc || parent == "" || parent != super || parent == owner {
		return "", false
	}
	if !c.sourceBridgeOriginalParentDeclaration(obj, parent, name, desc, resolve) {
		return "", false
	}
	return parent, true
}

// Source regeneration needs an inaccessible parent declaration, not merely a
// synthetic flag and a forwarder-shaped body. Keep only an unannotated public
// ordinary target with the identical erased descriptor and no throws/inference.
func (c *ClassObjectDumper) sourceBridgeOriginalParentDeclaration(owner *ClassObject, parent, name, descriptor string, resolve func(string) (*ClassObject, bool)) bool {
	p, known := resolve(parent)
	if !known || p == nil || p.GetClassName() != parent || p.AccessFlags&0x201 != 0 || len(p.Methods) > 4096 || !nativeProofWork(c.Work, int64(len(p.Methods))+1) {
		return false
	}
	pkg := func(n string) string {
		i := strings.LastIndexByte(n, '/')
		if i < 0 {
			return ""
		}
		return n[:i]
	}
	if pkg(owner.GetClassName()) != pkg(parent) {
		return false
	}
	var target *MemberInfo
	count := 0
	for _, m := range p.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, nok := sourceBridgeUTF8(p, m.NameIndex)
		d, dok := sourceBridgeUTF8(p, m.DescriptorIndex)
		if !nok || !dok || len(n) > 4096 || len(d) > 4096 || !nativeProofWork(c.Work, int64(len(n)+len(d))+1) {
			return false
		}
		if n == name && d == descriptor {
			count++
			target = m
		}
	}
	if count != 1 || target.AccessFlags != 1 || len(target.Attributes) > 128 {
		return false
	}
	codes := 0
	for _, a := range target.Attributes {
		if sourceProofNil(a) || !nativeProofWork(c.Work, 1) {
			return false
		}
		switch a.(type) {
		case *CodeAttribute:
			codes++
		case *SignatureAttribute:
		default:
			return false
		}
	}
	if codes != 1 {
		return false
	}
	sig, valid := nativeMethodLocalOriginalSignature(p, target.Attributes, c.Work)
	if !valid {
		return false
	}
	if sig == "" {
		return true
	}
	cs, valid := nativeMethodLocalOriginalSignature(p, p.Attributes, c.Work)
	if !valid || len(cs) > 4096 || len(sig) > 4096 || len(types.MethodFormalTypeParamNames(sig)) != 0 || !nativeProofWork(c.Work, int64(len(cs)+len(sig))*130+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(cs)+len(sig))*256+128) != nil {
		return false
	}
	erased, throws, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{cs}, sig)
	return valid && erased == descriptor && len(throws) == 0
}

// A witness must respect JVMS constant-pool tag constraints. The permissive
// legacy getUtf8 traverses other entries; using it here could certify malformed
// declarations (or recurse through a cyclic String entry) as real JVM evidence.
func sourceBridgeUTF8(obj *ClassObject, index uint16) (string, bool) {
	if obj == nil {
		return "", false
	}
	cp := NewConstantPoolWithConstant(&obj.ConstantPool)
	utf, ok := cp.IndexInfo(int(index)).(*ConstantUtf8Info)
	if !ok || utf == nil {
		return "", false
	}
	return utf.Value, true
}
func sourceBridgeClassName(obj *ClassObject, index uint16) (string, bool) {
	if obj == nil {
		return "", false
	}
	cp := NewConstantPoolWithConstant(&obj.ConstantPool)
	cl, ok := cp.IndexInfo(int(index)).(*ConstantClassInfo)
	if !ok || cl == nil {
		return "", false
	}
	return sourceBridgeUTF8(obj, cl.NameIndex)
}
