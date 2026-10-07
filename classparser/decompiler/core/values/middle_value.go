package values

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type JavaCompare struct {
	JavaValue1, JavaValue2 JavaValue
	// Original fcmp/dcmp result for an unordered pair: -1 for cmpl, +1
	// for cmpg. Zero belongs to ordinary ordered/reference comparisons.
	unorderedResult int8
}

// ReplaceVar implements JavaValue.
func (j *JavaCompare) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	j.JavaValue1.ReplaceVar(oldId, newId)
	j.JavaValue2.ReplaceVar(oldId, newId)
}

func (j *JavaCompare) Type() types.JavaType {
	return types.NewJavaPrimer(types.JavaBoolean)
}

func (j *JavaCompare) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s compare %s", j.JavaValue1.String(funcCtx), j.JavaValue2.String(funcCtx))
}

func NewJavaCompare(v1, v2 JavaValue) *JavaCompare {
	return &JavaCompare{
		JavaValue1: v1,
		JavaValue2: v2,
	}
}

// NewJavaFloatingCompare retains the instruction's unordered result rather than
// inferring it from the subsequently selected branch. Both operands are still
// evaluated exactly once, in their original left-to-right order.
func NewJavaFloatingCompare(v1, v2 JavaValue, unorderedLow bool) *JavaCompare {
	c := NewJavaCompare(v1, v2)
	c.unorderedResult = 1
	if unorderedLow {
		c.unorderedResult = -1
	}
	return c
}

// Predicate lowers a comparison of the JVM three-way result against zero. A
// float relation is false on NaN; its integer-opcode complement need not be.
// Use an explicit Boolean complement when unordered input satisfies the branch.
// This avoids re-evaluation through a separate isNaN test or a ternary compare.
func (j *JavaCompare) Predicate(op string) JavaValue {
	negate := false
	if j.unorderedResult < 0 {
		switch op {
		case LT:
			op, negate = GTE, true
		case LTE:
			op, negate = GT, true
		}
	} else if j.unorderedResult > 0 {
		switch op {
		case GT:
			op, negate = LTE, true
		case GTE:
			op, negate = LT, true
		}
	}
	result := JavaValue(NewBinaryExpression(j.JavaValue1, j.JavaValue2, op, types.NewJavaPrimer(types.JavaBoolean)))
	if negate {
		result = NewUnaryExpression(result, Not, types.NewJavaPrimer(types.JavaBoolean))
	}
	return result
}

type LambdaFuncRef struct {
	Id           int
	JavaType     types.JavaType
	LambdaRender func(funcCtx *class_context.ClassContext) string
	Arguments    []JavaValue
}

func (j *LambdaFuncRef) Type() types.JavaType {
	return j.JavaType
}

func (j *LambdaFuncRef) String(funcCtx *class_context.ClassContext) string {
	if j.LambdaRender != nil {
		return j.LambdaRender(funcCtx)
	}
	args := ""
	for _, arg := range j.Arguments {
		args += arg.String(funcCtx) + ","
	}
	return fmt.Sprintf("getLambda(%d)(%s)", j.Id, args)
}
