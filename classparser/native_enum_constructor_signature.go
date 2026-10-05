package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
)

// Descriptor parameters are physical JVM slots; Signature parameters describe
// the source declaration. Only an original genuine enum constructor with the
// exact String/int compiler prefix can have this two-argument difference.
// Count alone cannot justify lifting generics onto unrelated physical slots.
func (c *ClassObjectDumper) nativeEnumConstructorSignatureMatches(method *MemberInfo, signature, descriptor string) bool {
	if c == nil || c.obj == nil || c.obj.AccessFlags&0x4000 == 0 || c.obj.GetSupperClassName() != "java/lang/Enum" || method == nil || len(signature) > 4096 || !nativeProofWork(c.Work, int64(len(signature)+len(descriptor))) {
		return false
	}
	name, known := sourceBridgeUTF8(c.obj, method.NameIndex)
	actual, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !known || !dk || name != "<init>" || actual != descriptor || (method.AccessFlags != 2 && method.AccessFlags != 0x82) {
		return false
	}
	physical, result, e := callbinding.Descriptor(descriptor)
	if e != nil || result != "V" || len(physical) < 2 || physical[0] != "Ljava/lang/String;" || physical[1] != "I" {
		return false
	}
	erased, _, valid := types.EraseConcreteMethodSignatureWithThrows(signature)
	if !valid {
		erased, _, _, valid = types.UnboundedMethodErasure(signature)
	}
	if !valid {
		return false
	}
	source, ret, e := callbinding.Descriptor(erased)
	return e == nil && ret == "V" && slices.Equal(source, physical[2:])
}
