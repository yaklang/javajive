package values

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestBitwiseConstructionRetainsUnresolvedConstantSeed(t *testing.T) {
	for _, op := range []string{AND, OR, XOR} {
		for _, seed := range []int{0, 1} {
			for _, left := range []bool{false, true} {
				for _, numeric := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%d/left%v/numeric%v", op, seed, left, numeric), func(t *testing.T) {
						integer, boolean := types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaBoolean)
						slot := NewSlotValue(NewJavaLiteral(seed, integer), integer)
						rhs := NewCustomValue(func(*class_context.ClassContext) string { return "right()" }, func() types.JavaType { return boolean })
						var expression *JavaExpression
						if left {
							expression = NewBinaryExpression(slot, rhs, op, integer)
						} else {
							expression = NewBinaryExpression(rhs, slot, op, integer)
						}
						if _, ok := booleanWordConstant(slot); ok {
							t.Fatal("mutable forwarding seed is not a direct constant")
						}
						var resolved JavaValue
						if numeric {
							resolved = NewCustomValue(func(*class_context.ClassContext) string { return "word()" }, func() types.JavaType { return integer })
						} else {
							condition := NewCustomValue(func(*class_context.ClassContext) string { return "predicate()" }, func() types.JavaType { return boolean })
							resolved = NewTernaryExpression(condition, NewJavaLiteral(1, integer), NewJavaLiteral(0, integer))
						}
						slot.ResetValue(resolved)
						text := expression.String(&class_context.ClassContext{})
						want := "predicate()"
						if numeric {
							want = "word()"
						}
						if strings.Count(text, want) != 1 || strings.Count(text, "right()") != 1 {
							t.Fatalf("lost or duplicated operand: %s", text)
						}
						if strings.Index(text, want) < strings.Index(text, "right()") != left {
							t.Fatalf("operand order changed: %s", text)
						}
						kind := expression.Type().RawType().(*types.JavaPrimer).Name
						if numeric && kind != types.JavaInteger || !numeric && kind != types.JavaBoolean {
							t.Fatalf("domain=%s numeric=%v text=%s", kind, numeric, text)
						}
					})
				}
			}
		}
	}
}
