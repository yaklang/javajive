package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"math"
	"math/bits"
)

// Java8 inner classes may declare static constant variables, but not other
// static fields. Original ConstantValue evidence must both establish the JLS
// constant status and preserve the field's exact typed value when emitted.
func nativeMemberStaticConstantField(obj *ClassObject, field *MemberInfo, work *workbudget.Budget) bool {
	if obj == nil || field == nil || field.AccessFlags&0x18 != 0x18 || field.AccessFlags & ^uint16(0x009f) != 0 || bits.OnesCount16(field.AccessFlags&7) > 1 {
		return false
	}
	descriptor, known := sourceBridgeUTF8(obj, field.DescriptorIndex)
	if !known {
		return false
	}
	var constant *ConstantValueAttribute
	for _, attribute := range field.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if value, ok := attribute.(*ConstantValueAttribute); ok {
			if value == nil || constant != nil || value.AttrLen != 2 {
				return false
			}
			constant = value
		}
	}
	if constant == nil || constant.ConstantValueIndex < 1 || int(constant.ConstantValueIndex) > len(obj.ConstantPool) {
		return false
	}
	value := obj.ConstantPool[constant.ConstantValueIndex-1]
	switch descriptor {
	case "Z", "B", "S", "C", "I":
		n, ok := value.(*ConstantIntegerInfo)
		if !ok || n == nil {
			return false
		}
		switch descriptor {
		case "Z":
			return n.Value == 0 || n.Value == 1
		case "B":
			return n.Value >= -128 && n.Value <= 127
		case "S":
			return n.Value >= -32768 && n.Value <= 32767
		case "C":
			return n.Value >= 0 && n.Value <= 65535
		}
		return true
	case "J":
		n, ok := value.(*ConstantLongInfo)
		return ok && n != nil
	case "F":
		n, ok := value.(*ConstantFloatInfo)
		return ok && n != nil && (!math.IsNaN(float64(n.Value)) || math.Float32bits(n.Value) == 0x7fc00000)
	case "D":
		n, ok := value.(*ConstantDoubleInfo)
		return ok && n != nil && (!math.IsNaN(n.Value) || math.Float64bits(n.Value) == 0x7ff8000000000000)
	case "Ljava/lang/String;":
		n, ok := value.(*ConstantStringInfo)
		if !ok || n == nil {
			return false
		}
		_, known := sourceBridgeUTF8(obj, n.StringIndex)
		return known
	}
	return false
}
