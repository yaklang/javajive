package values

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func GetNotOpWithError(op string) (string, error) {
	switch op {
	case "==":
		return "!=", nil
	case "!=":
		return "==", nil
	case "<":
		return ">=", nil
	case ">=":
		return "<", nil
	case ">":
		return "<=", nil
	case "<=":
		return ">", nil
	default:
		return "", fmt.Errorf("[not support op %s]", op)
	}
}

func GetNotOp(op string) string {
	res, err := GetNotOpWithError(op)
	if err != nil {
		return err.Error()
	}
	return res
}
func SimplifyConditionValue(condition JavaValue) JavaValue {
	resVal := condition
	if val, ok := resVal.(*JavaExpression); ok {
		vals := []JavaValue{}
		for _, value := range val.Values {
			vals = append(vals, SimplifyConditionValue(value))
		}
		resVal = &JavaExpression{
			Op:     val.Op,
			Values: vals,
			Typ:    val.Typ,
		}
		if val.Op == Not {
			if v1, ok := vals[0].(*JavaExpression); ok {
				if v1.Op == Not {
					return v1.Values[0]
				} else {
					reverseOp, err := GetNotOpWithError(v1.Op)
					if err == nil {
						resVal = NewBinaryExpression(v1.Values[0], v1.Values[1], reverseOp, types.NewJavaPrimer(types.JavaBoolean))
					}
				}
			}
		}
	}
	return resVal
}

// branchConditionView is a use-site view of a predicate. JVM branch truth
// consumes the whole computational int word, unlike a Z storage sink. Late
// decision folding may expose an int producer here; never retype that producer
// or narrow its low bit just to satisfy Java's conditional operand syntax.
func branchConditionView(condition JavaValue) JavaValue {
	condition = SimplifyConditionValue(condition)
	if IsBooleanStackNarrowing(types.NewJavaPrimer(types.JavaBoolean), condition) {
		return NewBinaryExpression(condition, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), NEQ, types.NewJavaPrimer(types.JavaBoolean))
	}
	return condition
}
func UnpackSoltValue(value JavaValue) JavaValue {
	if ref, ok := value.(*SlotValue); ok {
		return UnpackSoltValue(ref.GetValue())
	}
	return value
}
