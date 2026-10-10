package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestRelationalNegationRequiresOrderedOperands(t *testing.T) {
	typeset := []string{types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble, "unknown"}
	for _, left := range typeset {
		for _, right := range typeset {
			for _, op := range []string{LT, LTE, GT, GTE, EQ, NEQ} {
				l := NewJavaLiteral(2, types.NewJavaPrimer(left))
				r := NewJavaLiteral(3, types.NewJavaPrimer(right))
				condition := NewBinaryExpression(l, r, op, types.NewJavaPrimer(types.JavaBoolean))
				neg := SimplifyConditionValue(NewUnaryExpression(condition, Not, types.NewJavaPrimer(types.JavaBoolean))).(*JavaExpression)
				ordered := func(s string) bool {
					switch s {
					case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong:
						return true
					}
					return false
				}
				complement := op == EQ || op == NEQ || ordered(left) && ordered(right)
				if complement {
					if neg.Op != GetNotOp(op) || len(neg.Values) != 2 || neg.Values[0] != l || neg.Values[1] != r {
						t.Fatalf("ordered complement %s/%s %s changed", left, right, op)
					}
				} else if neg.Op != Not || len(neg.Values) != 1 {
					t.Fatalf("unordered/unknown complement %s/%s %s erased", left, right, op)
				}
			}
		}
	}
}
