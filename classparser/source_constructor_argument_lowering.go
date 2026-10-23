package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// GETSTATIC may initialize a class, throw or read volatile storage. It is not
// treated as a pure expression or replaced by its current value. The original
// decoded field witness binds the source operand; the fresh typed packet must
// independently contain the same symbolic read in the same operand order.
// Moving the entire fresh SUPER packet behind the capture packet restores the
// original order of these effects, including abrupt exits before SUPER starts.
func constructorLoweringSourceArgument(argument constructorLoweringArgument, pc int, value values.JavaValue, slots map[int]int, params []values.JavaValue) bool {
	value = values.UnpackSoltValue(value)
	if argument.null {
		if argument.slot != 0 || argument.owner != "" || argument.name != "" || len(argument.nullCasts) > 32 {
			return false
		}
		descriptor := "Ljava/lang/Object;"
		if len(argument.nullCasts) > 0 {
			descriptor = argument.nullCasts[len(argument.nullCasts)-1]
		}
		if argument.descriptor != descriptor {
			return false
		}
		for i := len(argument.nullCasts) - 1; i >= 0; i-- {
			cast, ok := value.(*values.CastExpression)
			if !ok {
				return false
			}
			origin, target, known := cast.OriginalCheckCastWitness(&class_context.ClassContext{})
			if !known || origin != pc+1+3*i || target != argument.nullCasts[i] {
				return false
			}
			value = values.UnpackSoltValue(cast.Value)
		}
		literal, ok := value.(*values.JavaLiteral)
		if !ok {
			return false
		}
		origin, known := literal.OriginalNullPC()
		return known && origin == pc
	}
	if argument.owner != "" {
		field, ok := value.(*values.JavaClassMember)
		return ok && argument.slot == 0 && field.OriginalStaticFieldRead(pc, argument.owner, argument.name, argument.descriptor)
	}
	index, known := slots[argument.slot]
	return known && argument.slot > 0 && argument.name == "" && index >= 0 && index < len(params) && value == params[index]
}
