package rewriter

import (
	"github.com/yaklang/javajive/internal/jdecenv"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// NormalizeLoopForms rewrites two generic loop shapes after the CFG has been
// structured:
//
//  1. `do { if (cond) { body } else { break; } } while (true)` ->
//     `while (cond) { body }`.
//  2. `init; while (cond) { body; update; }` -> `for (init; cond; update) { body }`
//     when update is a simple increment/decrement of the same variable, init
//     does not introduce a variable that escapes the loop, and the body has no
//     unlabeled continue (a continue would skip a for-update but currently
//     executes the explicit tail update).
//
// Both rewrites preserve evaluation order and transfer targets; labeled loops
// and ambiguous shapes are left untouched.
func NormalizeLoopForms(sts []statements.Statement) []statements.Statement {
	if len(sts) == 0 || jdecenv.Get("JDEC_LOOP_FORM_NORMALIZE_OFF") != "" {
		return sts
	}
	normalized := make([]statements.Statement, 0, len(sts))
	for _, st := range sts {
		normalized = append(normalized, normalizeLoopStatement(st))
	}
	return convertForLikeLoops(normalized)
}

func normalizeLoopStatement(st statements.Statement) statements.Statement {
	if st == nil {
		return nil
	}
	switch s := st.(type) {
	case *statements.DoWhileStatement:
		s.Body = trimTrailingContinue(NormalizeLoopForms(s.Body))
		if while := doWhileAsWhile(s); while != nil {
			return while
		}
		return s
	case *statements.WhileStatement:
		s.Body = trimTrailingContinue(NormalizeLoopForms(s.Body))
		return s
	case *statements.ForStatement:
		if s.InitVar != nil {
			s.InitVar = normalizeLoopStatement(s.InitVar)
		}
		if s.EndExp != nil {
			s.EndExp = normalizeLoopStatement(s.EndExp)
		}
		s.SubStatements = trimTrailingContinue(NormalizeLoopForms(s.SubStatements))
		return s
	case *statements.IfStatement:
		s.IfBody = NormalizeLoopForms(s.IfBody)
		s.ElseBody = NormalizeLoopForms(s.ElseBody)
		return s
	case *statements.SwitchStatement:
		for _, item := range s.Cases {
			if item != nil {
				item.Body = NormalizeLoopForms(item.Body)
			}
		}
		return s
	case *statements.TryCatchStatement:
		s.TryBody = NormalizeLoopForms(s.TryBody)
		for i := range s.CatchBodies {
			s.CatchBodies[i] = NormalizeLoopForms(s.CatchBodies[i])
		}
		return s
	case *statements.SynchronizedStatement:
		s.Body = NormalizeLoopForms(s.Body)
		return s
	default:
		return st
	}
}

// trimTrailingContinue removes a source-level continue whose only effect is to
// jump to the loop continuation already reached by falling off the body. A
// labeled continue targets another loop and is retained.
func trimTrailingContinue(body []statements.Statement) []statements.Statement {
	if len(body) == 0 {
		return body
	}
	kind, label, known := sourceTransferKind(body[len(body)-1])
	if known && kind == "continue" && label == "" {
		return body[:len(body)-1]
	}
	return body
}

// doWhileAsWhile recognizes the canonical top-tested loop lowering. The
// condition is evaluated exactly once per iteration in both forms; `continue`
// inside the body still reaches the condition because the original do-while
// condition is the literal true and the guard sits at the body head.
func doWhileAsWhile(loop *statements.DoWhileStatement) *statements.WhileStatement {
	if loop == nil || loop.Label != "" || loop.ConditionValue == nil || !isTrueBooleanLiteral(loop.ConditionValue) ||
		len(loop.Body) != 1 {
		return nil
	}
	guard, ok := loop.Body[0].(*statements.IfStatement)
	if !ok || guard == nil || guard.Condition == nil || len(guard.IfBody) == 0 || len(guard.ElseBody) != 1 {
		return nil
	}
	kind, label, known := sourceTransferKind(guard.ElseBody[0])
	if !known || kind != "break" || label != "" {
		return nil
	}
	return &statements.WhileStatement{ConditionValue: guard.Condition, Body: trimTrailingContinue(guard.IfBody)}
}

func convertForLikeLoops(sts []statements.Statement) []statements.Statement {
	if len(sts) == 0 {
		return sts
	}
	out := make([]statements.Statement, 0, len(sts))
	for i, st := range sts {
		loop, ok := st.(*statements.WhileStatement)
		if !ok || len(out) == 0 {
			out = append(out, st)
			continue
		}
		init := out[len(out)-1]
		remaining := sts[i+1:]
		if forSt := makeForLoop(init, loop, remaining); forSt != nil {
			out[len(out)-1] = forSt
			continue
		}
		out = append(out, st)
	}
	return out
}

func makeForLoop(init statements.Statement, loop *statements.WhileStatement, remaining []statements.Statement) *statements.ForStatement {
	if init == nil || loop == nil || len(loop.Body) == 0 || loop.ConditionValue == nil {
		return nil
	}
	assign, ok := init.(*statements.AssignStatement)
	if !ok || assign == nil || assign.ArrayMember != nil {
		return nil
	}
	ref := plainLoopRef(assign.LeftValue)
	if ref == nil || ref.VarUid == "" {
		return nil
	}
	last := len(loop.Body) - 1
	update := loop.Body[last]
	if !isSimpleLoopUpdate(update, ref.VarUid) {
		return nil
	}
	body := loop.Body[:last]
	if hasUnlabeledContinue(body) {
		return nil
	}
	if (assign.IsFirst || assign.IsDeclare) && varUsedInStatements(remaining, ref.VarUid) {
		// A declaration moved into a for-init would leave the variable out of
		// scope for later siblings; a plain reassignment has no such scope.
		return nil
	}
	return &statements.ForStatement{
		InitVar:       init,
		Condition:     &statements.ConditionStatement{Condition: loop.ConditionValue},
		EndExp:        update,
		SubStatements: body,
	}
}

func plainLoopRef(v values.JavaValue) *values.JavaRef {
	for {
		switch t := v.(type) {
		case *values.SlotValue:
			if t == nil {
				return nil
			}
			v = t.GetValue()
		case *values.JavaRef:
			return t
		default:
			return nil
		}
	}
}

func isSimpleLoopUpdate(st statements.Statement, uid string) bool {
	if st == nil || uid == "" {
		return false
	}
	var expr values.JavaValue
	switch s := st.(type) {
	case *statements.ExpressionStatement:
		if s == nil {
			return false
		}
		expr = s.Expression
	case *values.JavaExpression:
		expr = s
	default:
		return false
	}
	es, ok := expr.(*values.JavaExpression)
	if !ok || es == nil || (es.Op != values.INC && es.Op != values.DEC) || len(es.Values) == 0 {
		return false
	}
	ref := plainLoopRef(es.Values[0])
	return ref != nil && ref.VarUid == uid
}

func hasUnlabeledContinue(sts []statements.Statement) bool {
	var walk func([]statements.Statement) bool
	var walkOne func(statements.Statement) bool
	walkOne = func(st statements.Statement) bool {
		if st == nil {
			return false
		}
		switch s := st.(type) {
		case *statements.DoWhileStatement, *statements.WhileStatement, *statements.ForStatement:
			// A continue in a nested loop targets that loop, not the candidate.
			return false
		case *statements.IfStatement:
			return walk(s.IfBody) || walk(s.ElseBody)
		case *statements.SwitchStatement:
			for _, item := range s.Cases {
				if item != nil && walk(item.Body) {
					return true
				}
			}
			return false
		case *statements.TryCatchStatement:
			if walk(s.TryBody) {
				return true
			}
			for _, body := range s.CatchBodies {
				if walk(body) {
					return true
				}
			}
			return false
		case *statements.SynchronizedStatement:
			return walk(s.Body)
		case *statements.CustomStatement:
			kind, label, known := s.SourceTransferProtocol()
			return known && kind == "continue" && label == ""
		default:
			return false
		}
	}
	walk = func(list []statements.Statement) bool {
		for _, st := range list {
			if walkOne(st) {
				return true
			}
		}
		return false
	}
	return walk(sts)
}

func varUsedInStatements(sts []statements.Statement, uid string) bool {
	if len(sts) == 0 || uid == "" {
		return false
	}
	used := false
	var walkValue func(values.JavaValue, int)
	walkValue = func(value values.JavaValue, depth int) {
		if used || value == nil || depth > 4096 {
			return
		}
		for {
			slot, ok := value.(*values.SlotValue)
			if !ok || slot == nil {
				break
			}
			value = slot.GetValue()
			if value == nil {
				return
			}
		}
		if ref, ok := value.(*values.JavaRef); ok {
			if ref != nil && ref.VarUid == uid {
				used = true
				return
			}
			if ref != nil && ref.CustomValue != nil {
				walkValue(ref.CustomValue, depth+1)
			}
			return
		}
		children, known := values.Children(value)
		if !known {
			return
		}
		for _, child := range children {
			walkValue(child, depth+1)
		}
	}
	walkStatementListValues(sts, func(value values.JavaValue) {
		walkValue(value, 0)
	})
	return used
}

func isTrueBooleanLiteral(v values.JavaValue) bool {
	lit, ok := v.(*values.JavaLiteral)
	if !ok || lit == nil {
		return false
	}
	value, ok := lit.Data.(bool)
	return ok && value
}

func sourceTransferKind(st statements.Statement) (string, string, bool) {
	cs, ok := st.(*statements.CustomStatement)
	if !ok || cs == nil {
		return "", "", false
	}
	return cs.SourceTransferProtocol()
}
