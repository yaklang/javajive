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
	if view, ok := BooleanStackConsumerView(value); ok {
		return view
	}
	// A rejected graph/category proof must not fall through to a raw Boolean
	// reduction. The dumper recognizes this marker and reports a stub.
	return NewCustomValue(func(*class_context.ClassContext) string { return EmptySlotValuePlaceholder }, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
}

// BooleanStackConsumerView narrows the selected ternary arm at a Z consumer,
// rather than retyping shared literals or changing the condition's nonzero
// semantics. Each selected arm is evaluated exactly once. Canonical Boolean
// operands already represent the correct low bit and need no conversion.
func BooleanStackConsumerView(value JavaValue) (JavaValue, bool) {
	memo := map[JavaValue]JavaValue{}
	expansion := map[JavaValue]int{}
	active := map[JavaValue]bool{}
	work := 0
	var visit func(JavaValue, int) (JavaValue, bool)
	visit = func(v JavaValue, depth int) (JavaValue, bool) {
		if v == nil || depth > 32 || active[v] {
			return nil, false
		}
		if result, ok := memo[v]; ok {
			return result, true
		}
		work++
		if work > 512 {
			return nil, false
		}
		active[v] = true
		defer delete(active, v)
		if slot, ok := v.(*SlotValue); ok {
			result, ok := visit(slot.val, depth+1)
			if ok {
				memo[v] = result
				expansion[v] = expansion[slot.val]
			}
			return result, ok
		}
		if ternary, ok := v.(*TernaryExpression); ok {
			if ternary.Condition == nil {
				return nil, false
			}
			first, ok := visit(ternary.TrueValue, depth+1)
			if !ok {
				return nil, false
			}
			second, ok := visit(ternary.FalseValue, depth+1)
			if !ok {
				return nil, false
			}
			// Memoization bounds analysis work, but a shared ternary DAG can
			// still expand exponentially when rendered. Bound both dimensions.
			size := 1 + expansion[ternary.TrueValue] + expansion[ternary.FalseValue]
			if size > 512 {
				return nil, false
			}
			result := NewTernaryExpression(ternary.Condition, first, second)
			memo[v] = result
			expansion[v] = size
			return result, true
		}
		if IsBooleanStackNarrowing(types.NewJavaPrimer(types.JavaBoolean), v) {
			result := narrowBooleanLeaf(v)
			memo[v] = result
			expansion[v] = 1
			return result, true
		}
		if isBooleanTyped(v) {
			if literal, ok := v.(*JavaLiteral); ok {
				switch literal.Data.(type) {
				case int, bool:
				default:
					return nil, false
				}
			}
			memo[v] = v
			expansion[v] = 1
			return v, true
		}
		return nil, false
	}
	return visit(value, 0)
}

func narrowBooleanLeaf(value JavaValue) JavaValue {
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
