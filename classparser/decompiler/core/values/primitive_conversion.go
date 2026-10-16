package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A JVM numeric conversion is a total primitive operation. Its operand can
// still throw. Keep the operation separate from an arbitrary formatting
// callback so effect-domain visitors can inspect that operand without rendering
// it. Public compatibility annotations are not its executable definition.
type sourcePrimitiveConversion struct {
	operand JavaValue
	target  string
	pc      int
}

func NewOriginalPrimitiveConversion(operand JavaValue, target types.JavaType, pc int) *CustomValue {
	if isNilJavaValue(operand) || target == nil || pc < 0 || pc > 65535 {
		return nil
	}
	primitive, ok := target.RawType().(*types.JavaPrimer)
	if !ok || primitive == nil {
		return nil
	}
	switch primitive.Name {
	case types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
	default:
		return nil
	}
	role := &sourcePrimitiveConversion{operand: operand, target: primitive.Name, pc: pc}
	return &CustomValue{sourcePrimitive: role, Flag: "primitive_cast", CapturesKnown: true, Captures: []JavaValue{operand}, OriginPC: pc, HasOriginPC: true,
		TypeFunc: func() types.JavaType { return types.NewJavaPrimer(role.target) }}
}

// SourcePrimitiveConversion describes the actual source operation. Consumers
// must also establish a primitive operand type and visit all of its effects.
// It deliberately does not trust Flag, Captures, TypeFunc or render callbacks.
func (v *CustomValue) SourcePrimitiveConversion() (operand JavaValue, target string, pc int, known bool) {
	if v == nil || v.sourcePrimitive == nil {
		return nil, "", 0, false
	}
	role := v.sourcePrimitive
	return role.operand, role.target, role.pc, true
}
