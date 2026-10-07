package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// ACC_SYNTHETIC and the Signature namespace are classfile-49 features. The
// straight accessor grammars have no version-dependent execution steps, but
// unused later constant-pool tags still make a pre-51 class invalid. A version
// edit alone must never manufacture evidence for a modern classfile feature.
// Versions53/54 still use this pre-nestmate field/accessor protocol. Module
// namespaces and version55 dynamic constants are outside these grammars.
func nativeAccessorVersion(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil || obj.MinorVersion != 0 || obj.MajorVersion < 49 || obj.MajorVersion > 54 {
		return false
	}
	for _, constant := range obj.ConstantPool {
		if constant != nil && sourceProofNil(constant) {
			return false
		}
		if !nativeProofWork(work, 1) {
			return false
		}
		switch constant.(type) {
		case nil, *ConstantUtf8Info, *ConstantIntegerInfo, *ConstantFloatInfo, *ConstantLongInfo, *ConstantDoubleInfo, *ConstantClassInfo, *ConstantStringInfo, *ConstantFieldrefInfo, *ConstantMethodrefInfo, *ConstantInterfaceMethodrefInfo, *ConstantNameAndTypeInfo:
		case *ConstantMethodHandleInfo, *ConstantMethodTypeInfo, *ConstantInvokeDynamicInfo:
			if obj.MajorVersion < 51 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
