package statements

import (
	"fmt"
	"math"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The integer result and branch oracle is independent of the source predicate
// builder. Cover both unordered choices, all six zero comparisons, every pair
// of IEEE boundary values, and repeated Boolean complementation.
func TestBoundedFloatingThreeWayPredicateModel(t *testing.T) {
	fs := []uint32{0, 0x80000000, 1, 0x80000001, 0x007fffff, 0x00800000, 0x3f800000, 0xbf800000, 0x7f7fffff, 0xff7fffff, 0x7f800000, 0xff800000, 0x7fc00000, 0x7fc01234, 0xffc01234, 0x3f800001}
	ds := []uint64{0, 0x8000000000000000, 1, 0x8000000000000001, 0x000fffffffffffff, 0x0010000000000000, 0x3ff0000000000000, 0xbff0000000000000, 0x7fefffffffffffff, 0xffefffffffffffff, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000000000, 0x7ff8000000001234, 0xfff8000000001234, 0x3ff0000000000001}
	ops := []string{"<", "<=", ">", ">=", "==", "!="}
	rel := func(a, b float64, op string) bool {
		switch op {
		case "<":
			return a < b
		case "<=":
			return a <= b
		case ">":
			return a > b
		case ">=":
			return a >= b
		case "==":
			return a == b
		case "!=":
			return a != b
		}
		panic("not a relation")
	}
	var eval func(values.JavaValue) (bool, bool)
	number := func(v values.JavaValue) float64 {
		l, ok := v.(*values.JavaLiteral)
		if !ok {
			panic(fmt.Sprintf("not a model number: %T", v))
		}
		switch n := l.Data.(type) {
		case float32:
			return float64(n)
		case float64:
			return n
		}
		panic("not a floating word")
	}
	eval = func(v values.JavaValue) (bool, bool) {
		e, ok := v.(*values.JavaExpression)
		if !ok {
			return false, false
		}
		if e.Op == "!" && len(e.Values) == 1 {
			b, known := eval(e.Values[0])
			return !b, known
		}
		if len(e.Values) != 2 {
			return false, false
		}
		return rel(number(e.Values[0]), number(e.Values[1]), e.Op), true
	}
	cases := 0
	for _, typ := range []string{types.JavaFloat, types.JavaDouble} {
		for i := range fs {
			for j := range fs {
				var da, db any
				var a, b float64
				if typ == types.JavaFloat {
					da, db = math.Float32frombits(fs[i]), math.Float32frombits(fs[j])
					a, b = float64(da.(float32)), float64(db.(float32))
				} else {
					da, db = math.Float64frombits(ds[i]), math.Float64frombits(ds[j])
					a, b = da.(float64), db.(float64)
				}
				for _, low := range []bool{false, true} {
					// Assign the comparison result through the independent IEEE relation table.
					result := 0
					if math.IsNaN(a) || math.IsNaN(b) {
						result = 1
						if low {
							result = -1
						}
					} else if a < b {
						result = -1
					} else if a > b {
						result = 1
					}
					for _, op := range ops {
						for neg := 0; neg < 3; neg++ {
							left := values.NewJavaLiteral(da, types.NewJavaPrimer(typ))
							right := values.NewJavaLiteral(db, types.NewJavaPrimer(typ))
							predicate := NewConditionStatement(values.NewJavaFloatingCompare(left, right, low), op).Condition
							expected := rel(float64(result), 0, op)
							for k := 0; k < neg; k++ {
								predicate = values.NewUnaryExpression(predicate, values.Not, types.NewJavaPrimer(types.JavaBoolean))
								expected = !expected
							}
							predicate = values.SimplifyConditionValue(predicate)
							got, known := eval(predicate)
							if !known || got != expected {
								t.Fatalf("%s pair%d/%d low%v op%s neg%d got%v known%v want%v", typ, i, j, low, op, neg, got, known, expected)
							}
							// Source complements may not add or reorder operand evaluations.
							var leaves []values.JavaValue
							var walk func(values.JavaValue)
							walk = func(v values.JavaValue) {
								if e, ok := v.(*values.JavaExpression); ok {
									for _, c := range e.Values {
										walk(c)
									}
								} else {
									leaves = append(leaves, v)
								}
							}
							walk(predicate)
							if len(leaves) != 2 || leaves[0] != left || leaves[1] != right {
								t.Fatalf("operand evaluation changed: %v", leaves)
							}
							cases++
						}
					}
				}
			}
		}
	}
	if cases != 18432 {
		t.Fatalf("model truncated: %d", cases)
	}
	t.Logf("independent IEEE three-way branch cases=%d", cases)
}
