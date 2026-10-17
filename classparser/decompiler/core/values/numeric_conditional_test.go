package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNumericConditionalPrimitivePromotionTable(t *testing.T) {
	kinds := []string{types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble}
	// JLS primitive numeric conditional table, with nonconstant operands.
	rows := []string{"BSIIJFD", "SSIIJFD", "IICIJFD", "IIIIJFD", "JJJJJFD", "FFFFFFD", "DDDDDDD"}
	names := map[byte]string{'B': types.JavaByte, 'S': types.JavaShort, 'C': types.JavaChar, 'I': types.JavaInteger, 'J': types.JavaLong, 'F': types.JavaFloat, 'D': types.JavaDouble}
	for i, a := range kinds {
		for j, b := range kinds {
			t.Run(a+"_"+b, func(t *testing.T) {
				av := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(a))
				bv := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(b))
				got, ok := NumericConditionalType(av, bv)
				if !ok || got.String(nil) != names[rows[i][j]] {
					t.Fatalf("%s/%s: %v,%v", a, b, got, ok)
				}
				if av.Type().String(nil) != a || bv.Type().String(nil) != b {
					t.Fatal("operand type mutated")
				}
			})
		}
	}
}
func TestNumericConditionalIntConstantBoundaries(t *testing.T) {
	for _, row := range []struct {
		kind      string
		low, high int
	}{{types.JavaByte, -128, 127}, {types.JavaShort, -32768, 32767}, {types.JavaChar, 0, 65535}} {
		for _, n := range []int{row.low - 1, row.low, row.high, row.high + 1} {
			for _, reverse := range []bool{false, true} {
				narrow := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(row.kind))
				var a, b JavaValue = narrow, NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger))
				if reverse {
					a, b = b, a
				}
				got, ok := NumericConditionalType(a, b)
				want := types.JavaInteger
				if n >= row.low && n <= row.high {
					want = row.kind
				}
				if !ok || got.String(nil) != want {
					t.Fatalf("%s/%d reverse=%v: got %v want %s", row.kind, n, reverse, got, want)
				}
			}
		}
	}
}
func TestNumericConditionalRejectsOtherDomainsAndRefreshesNumericCache(t *testing.T) {
	integer := NewJavaLiteral(-1, types.NewJavaPrimer(types.JavaInteger))
	for _, v := range []JavaValue{nil, NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Integer")), NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))} {
		if _, ok := NumericConditionalType(v, integer); ok {
			t.Fatal("non-numeric domain accepted")
		}
	}
	cycle := &SlotValue{}
	cycle.val = cycle
	if _, ok := NumericConditionalType(cycle, integer); ok {
		t.Fatal("cyclic slot accepted")
	}
	c := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaChar))
	ternary := &TernaryExpression{TrueValue: c, FalseValue: integer}
	ternary.SetCachedType(types.NewJavaPrimer(types.JavaChar))
	if got := ternary.Type(); got == nil || got.String(nil) != types.JavaInteger {
		t.Fatalf("stale narrow numeric cache: %v", got)
	}
	ternary.FalseValue = NewJavaLiteral(65535, types.NewJavaPrimer(types.JavaInteger))
	if ternary.Type().String(nil) != types.JavaChar {
		t.Fatal("resolved arm update ignored")
	}
	if c.Type().String(nil) != types.JavaChar {
		t.Fatal("conditional type polluted char producer")
	}
}
