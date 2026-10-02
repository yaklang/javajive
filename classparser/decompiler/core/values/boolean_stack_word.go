package values

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// booleanStackWord keeps JVM computational int words separate from Java Z
// operands. Enumerated captures retain effects and variable dependencies.
func booleanStackWord(value JavaValue) JavaValue {
	if condition, ok := boolOperandCondition(value); ok {
		value = condition
	}
	result := NewCustomValue(func(ctx *class_context.ClassContext) string {
		return fmt.Sprintf("((%s) ? 1 : 0)", value.String(ctx))
	}, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
	result.Flag, result.CapturesKnown, result.Captures = "boolean_stack_word", true, []JavaValue{value}
	result.ReplaceFunc = value.ReplaceVar
	return result
}

// IsBooleanStackNarrowing recognizes the int computational category at a Z
// consumer. The producer's type and the method's parameter ABI remain intact.
func IsBooleanStackNarrowing(target types.JavaType, value JavaValue) bool {
	if target == nil || value == nil || value.Type() == nil {
		return false
	}
	to, ok := target.RawType().(*types.JavaPrimer)
	if !ok || to.Name != types.JavaBoolean {
		return false
	}
	from, ok := value.Type().RawType().(*types.JavaPrimer)
	if !ok {
		return false
	}
	switch from.Name {
	case types.JavaInteger, types.JavaByte, types.JavaShort, types.JavaChar:
		return true
	}
	return false
}

// NarrowBooleanStackWord implements JVMS putfield/putstatic/ireturn Z
// narrowing: retain bit zero. Nonzero is incorrect for words such as 2 or -2.
// The operand is evaluated once, including any checks or side effects.
func NarrowBooleanStackWord(value JavaValue) JavaValue {
	if literal, ok := UnpackSoltValue(value).(*JavaLiteral); ok {
		if word, ok := literal.Data.(int); ok {
			return NewJavaLiteral(word&1, types.NewJavaPrimer(types.JavaBoolean))
		}
	}
	if condition, ok := BoolTernaryCondition(UnpackSoltValue(value)); ok {
		return condition
	}
	result := NewCustomValue(func(ctx *class_context.ClassContext) string {
		return fmt.Sprintf("(((%s) & 1) != 0)", value.String(ctx))
	}, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
	result.Flag, result.CapturesKnown, result.Captures = "boolean_stack_narrowing", true, []JavaValue{value}
	result.ReplaceFunc = value.ReplaceVar
	return result
}
