package core

import (
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
	if moving.Effects != 0 || len(moving.Writes) != 0 {
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
		crossed := statementAccess(cur.Statement)
		crossed.Handlers = d.handlersAt(op)
		if !values.CanSwap(moving, crossed) {
			return false
		}
	}
	return true
}
