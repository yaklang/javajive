package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Charge the physical payload before the lossless identity helper builds its
// display and encoded forms. A single long name, or many references to it,
// must consume the same request budget as other proof graph work.
func initializerIdentityUTF8Key(obj *ClassObject, index uint16, work *workbudget.Budget) (string, bool) {
	if obj == nil || index == 0 || int(index) > len(obj.ConstantPool) {
		return "", false
	}
	u, ok := obj.ConstantPool[index-1].(*ConstantUtf8Info)
	if !ok || u == nil || u.DecodeErr != nil {
		return "", false
	}
	size := int64(len(u.Units)) + int64(len(u.Value)) + 1
	if !nativeProofWork(work, size) || work.CheckAlloc(size*16) != nil {
		return "", false
	}
	return sourceParameterUTF8Key(obj, index)
}

func initializerIdentityClassKey(obj *ClassObject, index uint16, work *workbudget.Budget) (string, bool) {
	if obj == nil || index == 0 || int(index) > len(obj.ConstantPool) {
		return "", false
	}
	c, ok := obj.ConstantPool[index-1].(*ConstantClassInfo)
	if !ok || c == nil {
		return "", false
	}
	return initializerIdentityUTF8Key(obj, c.NameIndex, work)
}

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
	self, sk := initializerIdentityClassKey(obj, obj.ThisClass, work)
	target, tk := initializerIdentityClassKey(obj, ref.ClassIndex, work)
	n, nk := initializerIdentityUTF8Key(obj, pair.NameIndex, work)
	d, dk := initializerIdentityUTF8Key(obj, pair.DescriptorIndex, work)
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
		fn, fk := initializerIdentityUTF8Key(obj, f.NameIndex, work)
		fd, dk := initializerIdentityUTF8Key(obj, f.DescriptorIndex, work)
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
