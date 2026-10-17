package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A named lexical node need not own an enclosing instance. Member interfaces
// are static scope cuts, with no constructors or synthetic capture fields.
// Class and InnerClasses flags must describe the same declaration kind before
// the existing complete-forest, type-scope and source-layout proofs can apply.
func nativeMemberDeclarationKindRepresentable(obj *ClassObject, flags uint16, work *workbudget.Budget, resolvers ...func(string) (*ClassObject, bool)) bool {
	if obj == nil || flags&0x4000 != 0 {
		return false
	}
	if flags&0x2000 != 0 {
		return len(resolvers) == 1 && nativeMemberAnnotationDeclaration(obj, flags, work, resolvers[0])
	}
	if obj.AccessFlags&0x2000 != 0 {
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
	// Repeated physical descriptor indices share only immutable array-shape
	// facts. Every method still closes its own flags and Code attributes.
	var varargsDescriptors map[uint16]bool
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
		// Varargs describes the source calling convention of the final array
		// formal; it does not change abstract/default/static method kind or the
		// physical descriptor. Require that original array formal before stripping
		// this orthogonal modifier for the existing declaration-kind checks.
		if method.AccessFlags&0x0080 != 0 {
			if obj.MajorVersion < 49 {
				return false
			}
			// Descriptor parsing allocates a formal list. Account for that work
			// before parsing, including adversarial long CP strings and canceled
			// requests, rather than treating this new proof as a free flag mask.
			if !varargsDescriptors[method.DescriptorIndex] {
				if len(descriptor) > 65535 || !nativeProofWork(work, int64(len(descriptor))) || work != nil && work.CheckAlloc(int64(len(descriptor))*32+128+int64(len(varargsDescriptors)+1)*24) != nil {
					return false
				}
				params, _, err := callbinding.Descriptor(descriptor)
				if err != nil || len(params) == 0 || params[len(params)-1][0] != '[' {
					return false
				}
				if varargsDescriptors == nil {
					varargsDescriptors = map[uint16]bool{}
				}
				varargsDescriptors[method.DescriptorIndex] = true
			}
		}
		switch method.AccessFlags &^ uint16(0x0080) {
		case 0x0401:
			if codeCount != 0 {
				return false
			}
		case 0x0001, 0x0009:
			// Default and static methods remain legal after their introduction
			// in Java 8. Admit only versions with a proven original namespace.
			if obj.MajorVersion < 52 || !nativeAccessorVersion(obj, work) || codeCount != 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
