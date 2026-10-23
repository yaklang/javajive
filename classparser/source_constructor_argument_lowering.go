package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// GETSTATIC may initialize a class, throw or read volatile storage. It is not
// treated as a pure expression or replaced by its current value. The original
// decoded field witness binds the source operand; the fresh typed packet must
// independently contain the same symbolic read in the same operand order.
// Moving the entire fresh SUPER packet behind the capture packet restores the
// original order of these effects, including abrupt exits before SUPER starts.
func constructorLoweringSourceArgument(argument constructorLoweringArgument, pc int, value values.JavaValue, slots map[int]int, params []values.JavaValue) bool {
	value = values.UnpackSoltValue(value)
	if argument.owner != "" {
		field, ok := value.(*values.JavaClassMember)
		return ok && argument.slot == 0 && field.OriginalStaticFieldRead(pc, argument.owner, argument.name, argument.descriptor)
	}
	index, known := slots[argument.slot]
	return known && argument.slot > 0 && argument.name == "" && index >= 0 && index < len(params) && value == params[index]
}
