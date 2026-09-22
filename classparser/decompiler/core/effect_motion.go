package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func statementAccess(s statements.Statement) values.Access {
	switch v := s.(type) {
	case *statements.AssignStatement:
		target := v.LeftValue
		if v.ArrayMember != nil {
			target = v.ArrayMember
		}
		return values.InspectAccess(&values.AssignmentExpression{Target: target, Value: v.JavaValue})
	case *statements.ReturnStatement:
		return values.InspectAccess(v.JavaValue)
	case *statements.ExpressionStatement:
		return values.InspectAccess(v.Expression)
	case *statements.ConditionStatement:
		return values.InspectAccess(v.Condition)
	default:
		return values.Access{Effects: values.EffectOpaque}
	}
}
func (d *Decompiler) handlersAt(op *OpCode) []int {
	var out []int
	for i, h := range d.ExceptionTable {
		if op.CurrentOffset >= h.StartPc && op.CurrentOffset < h.EndPc {
			out = append(out, i)
		}
	}
	return out
}

// canInlineValue proves the complete proposed motion before graph or value
// mutation. A pure local read is checked against every crossed local write.
// Ambiguous paths, unavailable origins and changed handler coverage keep temps.
func (d *Decompiler) canInlineValue(value values.JavaValue, source, target *Node, origins map[int]*OpCode) bool {
	if source == nil || target == nil || source == target {
		return false
	}
	moving := values.InspectAccess(value)
	assignment, ok := source.Statement.(*statements.AssignStatement)
	if !ok {
		return false
	}
	ref, ok := values.UnpackSoltValue(assignment.LeftValue).(*values.JavaRef)
	if !ok || ref == nil {
		return false
	}
	origin := origins[source.Id]
	if origin == nil {
		return false
	}
	moving.Handlers = d.handlersAt(origin)
	cur := source
	seen := map[*Node]bool{}
	for cur != target {
		if seen[cur] || len(cur.Next) != 1 {
			return false
		}
		seen[cur] = true
		cur = cur.Next[0]
		if cur == nil || hasDistinctPredecessors(cur) {
			return false
		}
		if d.Work != nil {
			if err := d.Work.Charge(workbudget.CounterGraphEdges, 1); err != nil {
				return false
			}
		}
		op := origins[cur.Id]
		if op == nil {
			return false
		}
		if cur == target {
			prefix, ok := inlineUsePrefix(cur.Statement, ref)
			if !ok {
				return false
			}
			prefix.Handlers = d.handlersAt(op)
			return values.CanMoveAcrossPrefix(moving, prefix)
		}
		crossed := statementAccess(cur.Statement)
		crossed.Handlers = d.handlersAt(op)
		if !values.CanSwap(moving, crossed) {
			return false
		}
	}
	return true
}

// prefixBeforeUse computes Java's evaluated prefix before one value use. It
// rejects deferred/conditional uses, allocation boundaries and opaque closures.
func prefixBeforeUse(value values.JavaValue, ref *values.JavaRef) (values.Access, int, bool) {
	empty := func() values.Access {
		return values.Access{Reads: map[*values.JavaRef]bool{}, Writes: map[*values.JavaRef]bool{}}
	}
	seen := map[values.JavaValue]bool{}
	var walk func(values.JavaValue) (values.Access, int, bool)
	walk = func(v values.JavaValue) (values.Access, int, bool) {
		if v == nil {
			return empty(), 0, true
		}
		if seen[v] {
			return empty(), 0, false
		}
		seen[v] = true
		defer delete(seen, v)
		if r, ok := v.(*values.JavaRef); ok && values.SameLocal(r, ref) {
			return empty(), 1, true
		}
		var children []values.JavaValue
		conditional := false
		switch x := v.(type) {
		case *values.SlotValue:
			children = []values.JavaValue{x.GetValue()}
		case *values.FunctionCallExpression:
			children = append([]values.JavaValue{x.Object}, x.Arguments...)
		case *values.JavaExpression:
			children = x.Values
			conditional = x.Op == "&&" || x.Op == "||"
			// String + may perform conversion between operands.
			if x.Op == "+" && x.Type() != nil && x.Type().String(&class_context.ClassContext{}) == "String" {
				conditional = true
			}
		case *values.CastExpression:
			children = []values.JavaValue{x.Value}
		case *values.JavaArrayMember:
			children = []values.JavaValue{x.Object, x.Index}
		case *values.RefMember:
			children = []values.JavaValue{x.Object}
		case *values.JavaCompare:
			children = []values.JavaValue{x.JavaValue1, x.JavaValue2}
		case *values.TernaryExpression:
			children = []values.JavaValue{x.Condition, x.TrueValue, x.FalseValue}
			conditional = true
		default:
			a := values.InspectAccess(v)
			for r := range a.Reads {
				if values.SameLocal(r, ref) {
					return empty(), 0, false
				}
			}
			return a, 0, true
		}
		prefix := empty()
		count := 0
		for i, c := range children {
			a, n, ok := walk(c)
			if !ok {
				return empty(), 0, false
			}
			if n > 0 && conditional && i > 0 {
				return empty(), 0, false
			}
			if count == 0 {
				prefix.Effects |= a.Effects
				for r := range a.Reads {
					prefix.Reads[r] = true
				}
				for r := range a.Writes {
					prefix.Writes[r] = true
				}
			}
			count += n
		}
		if count == 0 {
			return values.InspectAccess(v), 0, true
		}
		return prefix, count, true
	}
	return walk(value)
}
func inlineUsePrefix(statement statements.Statement, ref *values.JavaRef) (values.Access, bool) {
	var expression values.JavaValue
	var lhsPrefix values.JavaValue
	switch s := statement.(type) {
	case *statements.ReturnStatement:
		expression = s.JavaValue
	case *statements.ExpressionStatement:
		expression = s.Expression
	case *statements.ConditionStatement:
		expression = s.Condition
	case *statements.AssignStatement:
		expression = s.JavaValue
		if s.ArrayMember != nil {
			lhsPrefix = values.NewJavaCompare(s.ArrayMember.Object, s.ArrayMember.Index)
		} else if target, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); !ok || target == nil {
			lhsPrefix = s.LeftValue
		}
	default:
		return values.Access{}, false
	}
	p, count, ok := prefixBeforeUse(expression, ref)
	if !ok || count != 1 {
		return values.Access{}, false
	}
	if lhsPrefix != nil {
		a := values.InspectAccess(lhsPrefix)
		p.Effects |= a.Effects
		for r := range a.Reads {
			p.Reads[r] = true
		}
		for r := range a.Writes {
			p.Writes[r] = true
		}
	}
	return p, true
}
