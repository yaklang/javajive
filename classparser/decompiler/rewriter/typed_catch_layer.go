package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Exception-table order establishes priority. A later handler protecting both
// the earlier region and its handler entry is an enclosing catch, not a sibling
// catch. Recover only a closed, fully covered rendering view; no handler body,
// exception type, value evaluation or original tree is discarded.
func RecoverCoveredTypedCatchLayer(tr *statements.TryCatchStatement) (*statements.TryCatchStatement, bool) {
	if tr == nil || len(tr.Exception) < 2 || len(tr.Exception) > 16 || len(tr.Exception) != len(tr.CatchBodies) || len(tr.Exception) != len(tr.Handlers) {
		return nil, false
	}
	last := len(tr.Handlers) - 1
	outer := tr.Handlers[last]
	if outer.CatchAll {
		ref := tr.Exception[last]
		if ref == nil {
			return nil, false
		}
		name, known := types.RawClassFQN(ref.Type())
		if !known || name != "java.lang.Throwable" {
			return nil, false
		}
	}
	if len(outer.ProtectedRanges) < 2 {
		return nil, false
	}
	covered := func(pc int) bool { return finallyContains(outer.ProtectedRanges, pc) }
	for _, handler := range tr.Handlers[:last] {
		if !covered(handler.EntryPC) || len(handler.ProtectedRanges) == 0 {
			return nil, false
		}
		for _, interval := range handler.ProtectedRanges {
			contained := false
			for _, region := range outer.ProtectedRanges {
				if region[0] <= interval[0] && interval[1] <= region[1] && interval[0] < interval[1] {
					contained = true
					break
				}
			}
			if !contained {
				return nil, false
			}
		}
	}
	proof := typedCatchCoverage{covered: covered, remaining: 512}
	if !proof.block(tr.TryBody) {
		return nil, false
	}
	for _, body := range tr.CatchBodies[:last] {
		if !proof.block(body) {
			return nil, false
		}
	}
	inner := *tr
	inner.Exception = append([]*values.JavaRef{}, tr.Exception[:last]...)
	inner.CatchBodies = append([][]statements.Statement{}, tr.CatchBodies[:last]...)
	inner.Handlers = append([]statements.CatchHandler{}, tr.Handlers[:last]...)
	out := *tr
	out.TryBody = []statements.Statement{&inner}
	out.Exception = []*values.JavaRef{tr.Exception[last]}
	out.CatchBodies = [][]statements.Statement{tr.CatchBodies[last]}
	out.Handlers = []statements.CatchHandler{outer}
	return &out, true
}

type typedCatchCoverage struct {
	covered   func(int) bool
	remaining int
}

func (p *typedCatchCoverage) block(body []statements.Statement) bool {
	for _, st := range body {
		p.remaining--
		if p.remaining < 0 || st == nil {
			return false
		}
		switch x := st.(type) {
		case *statements.ReturnStatement:
			if x.JavaValue != nil && !p.value(x.JavaValue, 0) {
				return false
			}
		case *statements.AssignStatement:
			if x == nil {
				return false
			}
			if x.JavaValue == nil {
				if _, local := x.LeftValue.(*values.JavaRef); !local || x.ArrayMember != nil {
					return false
				}
				continue
			}
			if !p.value(x.JavaValue, 0) {
				return false
			}
			if x.ArrayMember != nil {
				if !x.HasOriginPC || !p.covered(x.OriginPC) || !p.value(x.ArrayMember.Object, 0) || !p.value(x.ArrayMember.Index, 0) {
					return false
				}
			} else if _, local := x.LeftValue.(*values.JavaRef); !local {
				if !x.HasOriginPC || !p.covered(x.OriginPC) {
					return false
				}
				if field, ok := x.LeftValue.(*values.RefMember); ok && !p.value(field.Object, 0) {
					return false
				}
			}
		case *statements.ExpressionStatement:
			if !p.value(x.Expression, 0) {
				return false
			}
		case *statements.IfStatement:
			if !p.value(x.Condition, 0) {
				return false
			}
			if truth, known := pureBooleanGuardTruth(x.Condition, 0); known {
				live := x.IfBody
				if !truth {
					live = x.ElseBody
				}
				if !p.block(live) {
					return false
				}
			} else if !p.block(x.IfBody) || !p.block(x.ElseBody) {
				return false
			}
		case *statements.TryCatchStatement:
			if !p.block(x.TryBody) {
				return false
			}
			for _, b := range x.CatchBodies {
				if !p.block(b) {
					return false
				}
			}
		case *statements.CustomStatement:
			if x.Name == "end" {
				continue
			}
			if x.ThrownValue == nil || !x.HasOriginPC || !p.covered(x.OriginPC) || !p.value(x.ThrownValue, 0) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func (p *typedCatchCoverage) value(v values.JavaValue, depth int) bool {
	p.remaining--
	if p.remaining < 0 || depth > 32 {
		return false
	}
	v = plainTryValue(v)
	if v == nil {
		return false
	}
	if v == values.JavaNull {
		return true
	}
	var children []values.JavaValue
	switch x := v.(type) {
	case *values.JavaRef:
		return x != nil
	case *values.JavaLiteral:
		return x != nil
	case *values.JavaClassValue:
		// ldc class resolution may throw; this leaf carries no opcode PC.
		return false
	case *values.TernaryExpression:
		if x == nil {
			return false
		}
		children = []values.JavaValue{x.Condition, x.TrueValue, x.FalseValue}
	case *values.JavaExpression:
		if x == nil {
			return false
		}
		switch x.Op {
		case values.Not, values.EQ, values.NEQ, values.LOGICAL_AND, values.LOGICAL_OR:
		case values.ADD, values.SUB, values.MUL, values.AND, values.OR, values.XOR, values.SHL, values.SHR, values.USHR:
			// Total numeric operators carry no independent throw site. String
			// concatenation, boxing and integer division/remainder are excluded.
			if len(x.Values) != 2 && !(x.Op == values.SUB && len(x.Values) == 1) {
				return false
			}
			if !typedCatchNumericPrimitive(x.Typ) {
				return false
			}
			for _, v := range x.Values {
				if !typedCatchNumericValue(v) {
					return false
				}
			}
		case values.LT, values.LTE, values.GT, values.GTE:
			if len(x.Values) != 2 {
				return false
			}
			for _, v := range x.Values {
				if !typedCatchNumericValue(v) {
					return false
				}
			}
		default:
			return false
		}
		children = x.Values
	case *values.FunctionCallExpression:
		if x == nil || !x.HasOriginPC || !p.covered(x.OriginPC) || x.Kind >= values.InvokeDynamic || x.Descriptor == "" {
			return false
		}
		children = append(children, x.Arguments...)
		if !x.IsStatic {
			children = append(children, x.Object)
		}
	case *values.NewExpression:
		if x == nil || !x.HasOriginPC || !p.covered(x.OriginPC) {
			return false
		}
		if x.IsArray() {
			if len(x.Initializer) != 0 && (!x.HasEvaluationEndPC || !p.covered(x.EvaluationEndPC)) {
				return false
			}
			children = append(children, x.Length...)
			children = append(children, x.Initializer...)
		} else {
			ctor := x.ConstructorCall
			if ctor == nil || !ctor.HasOriginPC || !p.covered(ctor.OriginPC) || ctor.FunctionName != "<init>" {
				return false
			}
			children = ctor.Arguments
		}
	case *values.RefMember:
		if x == nil || !x.HasOriginPC || !p.covered(x.OriginPC) {
			return false
		}
		children = []values.JavaValue{x.Object}
	case *values.JavaClassMember:
		return x != nil && x.HasOriginPC && p.covered(x.OriginPC)
	case *values.CustomValue:
		// JVM numeric conversions are total. The closure remains opaque unless
		// its decoder-owned category, complete single capture and both numeric
		// types establish that only the captured operand can throw.
		if x == nil || x.Flag != "primitive_cast" || !x.CapturesKnown || len(x.Captures) != 1 || x.TypeFunc == nil || !typedCatchNumericPrimitive(x.Type()) {
			return false
		}
		operand := plainTryValue(x.Captures[0])
		if !typedCatchNumericValue(operand) {
			return false
		}
		children = []values.JavaValue{operand}
	case *values.CastExpression:
		if x == nil || !x.OriginalCheckCast || x.OriginPC < 0 || !p.covered(x.OriginPC) {
			return false
		}
		children = []values.JavaValue{x.Value}
	default:
		return false
	}
	for _, child := range children {
		if !p.value(child, depth+1) {
			return false
		}
	}
	return true
}

// Only literal boolean trees establish an unreachable arm. A ref's mutable Val,
// field read, call or rendered spelling supplies no constant/effect evidence.
func pureBooleanGuardTruth(v values.JavaValue, depth int) (bool, bool) {
	if depth > 16 {
		return false, false
	}
	v = plainTryValue(v)
	switch x := v.(type) {
	case *values.JavaLiteral:
		if x == nil {
			return false, false
		}
	case *values.JavaExpression:
		if x == nil {
			return false, false
		}
	default:
		return false, false
	}
	if v == nil || v.Type() == nil {
		return false, false
	}
	p, ok := v.Type().RawType().(*types.JavaPrimer)
	if !ok || p == nil || p.Name != types.JavaBoolean {
		return false, false
	}
	if literal, ok := v.(*values.JavaLiteral); ok && literal != nil {
		b, ok := literal.Data.(bool)
		return b, ok
	}
	x, ok := v.(*values.JavaExpression)
	if !ok || x == nil {
		return false, false
	}
	if x.Op == values.Not && len(x.Values) == 1 {
		b, ok := pureBooleanGuardTruth(x.Values[0], depth+1)
		return !b, ok
	}
	if (x.Op == values.LOGICAL_AND || x.Op == values.LOGICAL_OR || x.Op == values.EQ || x.Op == values.NEQ) && len(x.Values) == 2 {
		a, ak := pureBooleanGuardTruth(x.Values[0], depth+1)
		b, bk := pureBooleanGuardTruth(x.Values[1], depth+1)
		if !ak || !bk {
			return false, false
		}
		switch x.Op {
		case values.LOGICAL_AND:
			return a && b, true
		case values.LOGICAL_OR:
			return a || b, true
		case values.EQ:
			return a == b, true
		case values.NEQ:
			return a != b, true
		}
	}
	return false, false
}

func typedCatchNumericPrimitive(t types.JavaType) bool {
	if t == nil {
		return false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	if !ok || p == nil {
		return false
	}
	switch p.Name {
	case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
		return true
	}
	return false
}

func typedCatchNumericValue(v values.JavaValue) bool {
	v = plainTryValue(v)
	var t types.JavaType
	switch x := v.(type) {
	case *values.JavaRef:
		if x != nil {
			t = x.Type()
		}
	case *values.JavaLiteral:
		if x != nil {
			t = x.Type()
		}
	case *values.JavaExpression:
		if x != nil {
			t = x.Typ
		}
	case *values.FunctionCallExpression:
		if x != nil && x.FuncType != nil {
			t = x.FuncType.ReturnType
		}
	case *values.CustomValue:
		if x != nil && x.TypeFunc != nil {
			t = x.Type()
		}
	case *values.CastExpression:
		if x != nil {
			t = x.TargetType
		}
	case *values.RefMember:
		if x != nil {
			t = x.JavaType
		}
	case *values.JavaClassMember:
		if x != nil {
			t = x.JavaType
		}
	}
	return typedCatchNumericPrimitive(t)
}
