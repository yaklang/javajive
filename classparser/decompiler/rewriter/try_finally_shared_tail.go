package rewriter

import (
	"math"
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// FinallyConsumesSharedReturn proves that structuring has already copied
// the same decoded return and inert value into every normal try/catch completion.
// The original shared node still follows the region in the IR, but Java would
// reject that duplicate as unreachable. Only suppress its rendering after the
// complete original finally-domain/effect proof succeeds. Retain the IR and all
// real evaluations; unknown values, fallthrough and different-PC exits fail closed.
func FinallyConsumesSharedReturn(tr *statements.TryCatchStatement, tail *statements.ReturnStatement) bool {
	if tail == nil || !sameSharedReturnLiteral(tail.JavaValue, tail.JavaValue) || !tail.HasOriginPC || tail.OriginPC < 0 {
		return false
	}
	region, ok := RecoverCatchAllFinally(tr)
	if !ok {
		return false
	}
	for _, handler := range tr.Handlers {
		if finallyContains(handler.ProtectedRanges, tail.OriginPC) {
			return false
		}
	}
	remaining := 256
	var returns func([]statements.Statement, int) bool
	returns = func(body []statements.Statement, depth int) bool {
		if depth > 16 {
			return false
		}
		body = finallyWithoutEnd(body)
		if len(body) == 0 {
			return false
		}
		remaining--
		if remaining < 0 {
			return false
		}
		switch last := body[len(body)-1].(type) {
		case *statements.ReturnStatement:
			return last != nil && last.HasOriginPC && last.OriginPC == tail.OriginPC && sameSharedReturnLiteral(last.JavaValue, tail.JavaValue)
		case *statements.IfStatement:
			return last != nil && last.Condition != nil && returns(last.IfBody, depth+1) && returns(last.ElseBody, depth+1)
		case *statements.TryCatchStatement:
			// The enclosing finally proof has already verified these nested
			// handler domains and cleanup copies. A caught abrupt exit can still
			// resume normally, so every handler must prove its own terminal.
			if last == nil || len(last.Exception) != len(last.CatchBodies) || len(last.Handlers) != len(last.CatchBodies) || !returns(last.TryBody, depth+1) {
				return false
			}
			for _, body := range last.CatchBodies {
				if !returns(body, depth+1) {
					return false
				}
			}
			return true
		case *statements.CustomStatement:
			// A proved throw has no normal continuation. Text that happens to
			// look like a throw is not an authenticated control-flow edge.
			_, known := last.SourceThrowOperand()
			return known && last.HasOriginPC && last.OriginPC >= 0
		default:
			return false
		}
	}
	if !returns(region.TryBody, 0) {
		return false
	}
	for _, body := range region.CatchBodies {
		if !returns(body, 0) {
			return false
		}
	}
	return true
}

// The shared value is an actual literal, not a local that cleanup can mutate
// or a call/field access that might be evaluated again. Preserve literal type,
// UTF-16 units and floating-point bits instead of comparing rendered text.
func sameSharedReturnLiteral(a, b values.JavaValue) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a == values.JavaNull || b == values.JavaNull {
		return a == values.JavaNull && b == values.JavaNull
	}
	x, xok := a.(*values.JavaLiteral)
	y, yok := b.(*values.JavaLiteral)
	if !xok || !yok || x == nil || y == nil || x.Type() == nil || y.Type() == nil || !reflect.DeepEqual(x.Type().RawType(), y.Type().RawType()) || !reflect.DeepEqual(x.Units, y.Units) {
		return false
	}
	// ACONST_NULL is not the text "null": require its original producer.
	if pc, known := x.OriginalNullPC(); known {
		other, known := y.OriginalNullPC()
		return known && pc == other
	}
	primitive, isPrimitive := x.Type().RawType().(*types.JavaPrimer)
	name := ""
	if isPrimitive && primitive != nil {
		name = primitive.Name
	}
	stringClass, _ := types.RawClassFQN(x.Type())
	if name == types.JavaString || stringClass == "java.lang.String" {
		// The decoder uses the String primer; other producers use its FQN.
		// Units remain authoritative for lone surrogates, including an empty
		// non-nil slice. Never infer a string from the rendered spelling.
		switch x.Data.(type) {
		case string, []uint16:
			return reflect.DeepEqual(x.Data, y.Data)
		default:
			return x.Data == nil && y.Data == nil && x.Units != nil
		}
	}
	if x.Units != nil || !isPrimitive {
		return false
	}
	switch data := x.Data.(type) {
	case int, int8, int16, int32, int64, uint8, uint16, uint32, uint64:
		return (name == types.JavaInteger || name == types.JavaLong || name == types.JavaByte || name == types.JavaShort || name == types.JavaChar || name == types.JavaBoolean) && reflect.DeepEqual(x.Data, y.Data)
	case bool:
		return name == types.JavaBoolean && reflect.DeepEqual(x.Data, y.Data)
	case float32:
		other, ok := y.Data.(float32)
		return name == types.JavaFloat && ok && math.Float32bits(data) == math.Float32bits(other)
	case float64:
		other, ok := y.Data.(float64)
		return name == types.JavaDouble && ok && math.Float64bits(data) == math.Float64bits(other)
	default:
		return false
	}
}
