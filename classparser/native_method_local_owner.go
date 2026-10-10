package javaclassparser

import (
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Method locals have a distinct lexical role. These immutable declaration
// facts grant neither private access nor source ownership of a generated body.
// A later joint source transaction must prove its captures and placement.
type nativeMethodLocalOwner struct {
	owner, method, descriptor, name string
	ordinal                         int
	flags                           uint16
	declaration                     *MemberInfo
}

func originalMethodLocalOwner(obj, enclosing *ClassObject, work *workbudget.Budget) (*nativeMethodLocalOwner, bool) {
	if obj == nil || enclosing == nil || obj.MajorVersion < 49 || !nativeProofWork(work, 1) {
		return nil, false
	}
	proof := &nativeMethodLocalOwner{}
	attributes, selfRows := 0, 0
	for _, attribute := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if raw, ok := attribute.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" {
			attributes++
			if attributes != 1 || raw.Length != 4 || len(raw.Info) != 4 {
				return nil, false
			}
			owner, known := sourceBridgeClassName(obj, binary.BigEndian.Uint16(raw.Info[:2]))
			index := int(binary.BigEndian.Uint16(raw.Info[2:]))
			if !known || owner != enclosing.GetClassName() || index < 1 || index > len(obj.ConstantPool) {
				return nil, false
			}
			table, ok := obj.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
			if !ok || table == nil {
				return nil, false
			}
			name, named := sourceBridgeUTF8(obj, table.NameIndex)
			descriptor, typed := sourceBridgeUTF8(obj, table.DescriptorIndex)
			_, ret, err := callbinding.Descriptor(descriptor)
			if !named || !typed || err != nil || ret == "" || name == "" || name == "<clinit>" || name != "<init>" && class_context.SafeIdentifier(name) != name || name == "<init>" && ret != "V" {
				return nil, false
			}
			proof.owner, proof.method, proof.descriptor = owner, name, descriptor
		}
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil {
				return nil, false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return nil, false
				}
				self, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known {
					return nil, false
				}
				if self != obj.GetClassName() {
					continue
				}
				selfRows++
				name, known := sourceBridgeUTF8(obj, row.InnerNameIndex)
				if selfRows != 1 || row.OuterClassInfoIndex != 0 || !known || name == "" || class_context.SafeIdentifier(name) != name || row.InnerClassAccessFlags&0x0608 != 0 {
					return nil, false
				}
				proof.name, proof.flags = name, row.InnerClassAccessFlags
			}
		}
	}
	if attributes != 1 || selfRows != 1 {
		return nil, false
	}
	// The actual class and exact method must corroborate the EnclosingMethod CP
	// tuple. Matching a method name, a $ spelling, or a foreign self row is not
	// a lexical ownership certificate.
	for _, m := range enclosing.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, named := sourceBridgeUTF8(enclosing, m.NameIndex)
		desc, typed := sourceBridgeUTF8(enclosing, m.DescriptorIndex)
		if !named || !typed {
			return nil, false
		}
		if name == proof.method && desc == proof.descriptor {
			if proof.declaration != nil {
				return nil, false
			}
			proof.declaration = m
		}
	}
	if proof.declaration == nil {
		return nil, false
	}
	corroborated := 0
	for _, a := range enclosing.Attributes {
		if table, ok := a.(*InnerClassesAttribute); ok {
			if table == nil {
				return nil, false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return nil, false
				}
				name, known := sourceBridgeClassName(enclosing, row.InnerClassInfoIndex)
				if !known {
					return nil, false
				}
				if name != obj.GetClassName() {
					continue
				}
				corroborated++
				simple, known := sourceBridgeUTF8(enclosing, row.InnerNameIndex)
				if corroborated != 1 || !known || simple != proof.name || row.OuterClassInfoIndex != 0 || row.InnerClassAccessFlags != proof.flags {
					return nil, false
				}
			}
		}
	}
	if corroborated != 1 {
		return nil, false
	}
	// Only after original attribute ownership is closed, check whether javac can
	// regenerate this named-local binary registration. This spelling never
	// creates an owner or licenses an unknown local declaration.
	full := obj.GetClassName()
	prefix := proof.owner + "$"
	if len(full) <= len(prefix)+len(proof.name) || !strings.HasPrefix(full, prefix) || !strings.HasSuffix(full, proof.name) {
		return nil, false
	}
	digits := full[len(prefix) : len(full)-len(proof.name)]
	if digits == "" || digits[0] == '0' || len(digits) > 9 {
		return nil, false
	}
	for _, b := range digits {
		if b < '0' || b > '9' {
			return nil, false
		}
	}
	ordinal, err := strconv.Atoi(digits)
	if err != nil || ordinal <= 0 {
		return nil, false
	}
	proof.ordinal = ordinal
	return proof, true
}
