package values

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

type originalNull struct {
	literal *JavaLiteral
	pc      int
}

// Record the actual ACONST_NULL value, rather than granting a producer origin
// to an arbitrary literal whose rendered spelling happens to be "null".
func NewOriginalNullLiteral(pc int) *JavaLiteral {
	literal := NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
	if pc >= 0 && pc <= 65535 {
		literal.originalNull = &originalNull{literal: literal, pc: pc}
	}
	return literal
}

func (literal *JavaLiteral) OriginalNullPC() (int, bool) {
	if literal == nil || literal.originalNull == nil || literal.originalNull.literal != literal || literal.Units != nil {
		return 0, false
	}
	data, known := literal.Data.(string)
	if !known || data != "null" {
		return 0, false
	}
	name, known := types.RawClassFQN(literal.JavaType)
	if !known || name != "java.lang.Object" && name != "java/lang/Object" {
		return 0, false
	}
	return literal.originalNull.pc, true
}
