package values

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

type originalIntegerTrap struct {
	expression           *JavaExpression
	pc                   int
	operator, descriptor string
	operands             [2]JavaValue
}

// Preserve the actual typed JVM divide/remainder producer. Its abrupt event
// must remain after both original operands, independently of source spelling.
// Other expression constructors cannot fabricate a physical trap location.
func NewOriginalIntegerTrapExpression(left, right JavaValue, operator string, typ types.JavaType, pc int) *JavaExpression {
	if isNilJavaValue(left) || isNilJavaValue(right) {
		return &JavaExpression{Values: []JavaValue{left, right}, Op: operator, Typ: typ}
	}
	expression := NewBinaryExpression(left, right, operator, typ)
	descriptor, known := integerTrapDescriptor(typ)
	if pc >= 0 && pc <= 65535 && known && (operator == DIV || operator == REM) && len(expression.Values) == 2 && !isNilJavaValue(left) && !isNilJavaValue(right) {
		expression.originalIntegerTrap = &originalIntegerTrap{expression: expression, pc: pc, operator: operator, descriptor: descriptor, operands: [2]JavaValue{expression.Values[0], expression.Values[1]}}
	}
	return expression
}

func integerTrapDescriptor(typ types.JavaType) (string, bool) {
	if typ == nil {
		return "", false
	}
	primitive, known := typ.RawType().(*types.JavaPrimer)
	if !known || primitive == nil {
		return "", false
	}
	switch primitive.Name {
	case types.JavaInteger:
		return "I", true
	case types.JavaLong:
		return "J", true
	}
	return "", false
}

func (expression *JavaExpression) OriginalIntegerTrap() (pc int, operator, descriptor string, known bool) {
	if expression == nil || expression.originalIntegerTrap == nil {
		return
	}
	original := expression.originalIntegerTrap
	actual, typed := integerTrapDescriptor(expression.Typ)
	if original.expression != expression || len(expression.Values) != 2 || expression.Op != original.operator || !typed || actual != original.descriptor || expression.Values[0] != original.operands[0] || expression.Values[1] != original.operands[1] {
		return
	}
	for _, operand := range expression.Values {
		if isNilJavaValue(operand) || !integerTrapOperandWidth(operand.Type(), actual) {
			return
		}
	}
	return original.pc, original.operator, original.descriptor, true
}

// Java's binary numeric promotion agrees with the physical JVM word width.
// A later source-type edit must not turn an original int operation into long
// division or an original long operation into floating arithmetic.
func integerTrapOperandWidth(typ types.JavaType, descriptor string) bool {
	if typ == nil {
		return false
	}
	primitive, known := typ.RawType().(*types.JavaPrimer)
	if !known || primitive == nil {
		return false
	}
	if descriptor == "J" {
		return primitive.Name == types.JavaLong
	}
	return descriptor == "I" && (primitive.Name == types.JavaInteger || primitive.Name == types.JavaByte || primitive.Name == types.JavaShort || primitive.Name == types.JavaChar)
}

// A damaged integer witness cannot be reclassified as non-trapping floating arithmetic.
func (expression *JavaExpression) HasIntegerTrapOrigin() bool {
	return expression != nil && expression.originalIntegerTrap != nil
}
