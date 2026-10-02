package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
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
	if outer.CatchAll || len(outer.ProtectedRanges) < 2 {
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
			if !p.value(x.Condition, 0) || !p.block(x.IfBody) || !p.block(x.ElseBody) {
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
	case *values.JavaRef, *values.JavaLiteral, *values.JavaClassValue:
		return true
	case *values.TernaryExpression:
		children = []values.JavaValue{x.Condition, x.TrueValue, x.FalseValue}
	case *values.JavaExpression:
		switch x.Op {
		case values.Not, values.EQ, values.NEQ, values.LOGICAL_AND, values.LOGICAL_OR:
		default:
			return false
		}
		children = x.Values
	case *values.FunctionCallExpression:
		if !x.HasOriginPC || !p.covered(x.OriginPC) || x.Kind >= values.InvokeDynamic || x.Descriptor == "" {
			return false
		}
		children = append(children, x.Arguments...)
		if !x.IsStatic {
			children = append(children, x.Object)
		}
	case *values.NewExpression:
		if !x.HasOriginPC || !p.covered(x.OriginPC) {
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
		if !x.HasOriginPC || !p.covered(x.OriginPC) {
			return false
		}
		children = []values.JavaValue{x.Object}
	case *values.JavaClassMember:
		return x.HasOriginPC && p.covered(x.OriginPC)
	case *values.CastExpression:
		if !p.covered(x.OriginPC) {
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
