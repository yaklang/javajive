package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Source views are not physical member identities. Require the original
// Fieldref's exact UTF-16 owner/name/descriptor to identify one own static
// blank-final declaration. Only that store can move across the try boundary
// without introducing or losing a field-resolution/class-initialization edge.
func originalOwnStaticFinalStore(obj *ClassObject, method *MemberInfo, code *CodeAttribute, pc int, owner, name, descriptor string, work *workbudget.Budget) bool {
	if obj == nil || method == nil || code == nil || constructorLoweringCode(method) != code || method.AccessFlags != 8 || pc < 0 || pc+3 > len(code.Code) || code.Code[pc] != 0xb3 || obj.GetClassName() != owner {
		return false
	}
	mn, mk := sourceBridgeUTF8(obj, method.NameIndex)
	md, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
	if !mk || !dk || mn != "<clinit>" || md != "()V" {
		return false
	}
	index := binary.BigEndian.Uint16(code.Code[pc+1:])
	if index == 0 || int(index) > len(obj.ConstantPool) {
		return false
	}
	ref, ok := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
	if !ok || ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
		return false
	}
	pair, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || pair == nil {
		return false
	}
	self, sk := sourceParameterClassKey(obj, obj.ThisClass)
	target, tk := sourceParameterClassKey(obj, ref.ClassIndex)
	n, nk := sourceParameterUTF8Key(obj, pair.NameIndex)
	d, dk := sourceParameterUTF8Key(obj, pair.DescriptorIndex)
	viewName, vn := sourceBridgeUTF8(obj, pair.NameIndex)
	viewDesc, vd := sourceBridgeUTF8(obj, pair.DescriptorIndex)
	if !sk || !tk || self != target || !nk || !dk || !vn || !vd || viewName != name || viewDesc != descriptor {
		return false
	}
	matches := 0
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return false
		}
		fn, fk := sourceParameterUTF8Key(obj, f.NameIndex)
		fd, dk := sourceParameterUTF8Key(obj, f.DescriptorIndex)
		if !fk || !dk {
			return false
		}
		if fn != n || fd != d {
			continue
		}
		if f.AccessFlags&0x58 != 0x18 {
			return false
		}
		for _, a := range f.Attributes {
			if _, constant := a.(*ConstantValueAttribute); constant {
				return false
			}
		}
		matches++
	}
	return matches == 1
}
