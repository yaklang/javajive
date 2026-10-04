package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A named lexical node need not own an enclosing instance. Member interfaces
// are static scope cuts, with no constructors or synthetic capture fields.
// Class and InnerClasses flags must describe the same declaration kind before
// the existing complete-forest, type-scope and source-layout proofs can apply.
func nativeMemberDeclarationKindRepresentable(obj *ClassObject, flags uint16, work *workbudget.Budget) bool {
	if obj == nil || flags&(0x2000|0x4000) != 0 {
		return false
	}
	if flags&0x0200 == 0 {
		return obj.AccessFlags & ^uint16(0x0431) == 0
	}
	if flags&0x0608 != 0x0608 || flags & ^uint16(0x060f) != 0 || obj.AccessFlags & ^uint16(0x0601) != 0 || obj.AccessFlags&0x0600 != 0x0600 || obj.GetSupperClassName() != "java/lang/Object" {
		return false
	}
	for _, field := range obj.Fields {
		if field == nil || !nativeProofWork(work, 1) || field.AccessFlags != 0x0019 {
			return false
		}
	}
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, known := sourceBridgeUTF8(obj, method.NameIndex)
		if !known || name == "<init>" {
			return false
		}
		descriptor, known := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !known {
			return false
		}
		codeCount := 0
		for _, attribute := range method.Attributes {
			if !nativeProofWork(work, 1) {
				return false
			}
			if code, ok := attribute.(*CodeAttribute); ok {
				if code == nil {
					return false
				}
				codeCount++
			}
		}
		if name == "<clinit>" {
			if method.AccessFlags != 8 || descriptor != "()V" || codeCount != 1 {
				return false
			}
			continue
		}
		switch method.AccessFlags {
		case 0x0401:
			if codeCount != 0 {
				return false
			}
		case 0x0001, 0x0009:
			if obj.MajorVersion != 52 || codeCount != 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
