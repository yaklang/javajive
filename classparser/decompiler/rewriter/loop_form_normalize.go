package rewriter

import (
	"github.com/yaklang/javajive/internal/jdecenv"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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
	for i, st := range sts {
		normalizedSt := normalizeLoopStatement(st)
		if dw, ok := normalizedSt.(*statements.DoWhileStatement); ok && jdecenv.Get("JDEC_LOOP_NORM_EXT_OFF") == "" {
			if prefix, whileSt := doWhileWithConstPrefixAsWhile(dw, sts[i+1:]); whileSt != nil {
				if len(normalized) > 0 {
					if prevAssign, ok := normalized[len(normalized)-1].(*statements.AssignStatement); ok && prevAssign != nil &&
						prevAssign.ArrayMember == nil && plainLoopRef(prevAssign.LeftValue) != nil &&
						makeForLoop(prevAssign, whileSt, sts[i+1:]) != nil {
						var conflict bool
						for _, p := range prefix {
							if pa, ok := p.(*statements.AssignStatement); ok {
								pr := plainLoopRef(pa.LeftValue)
								if pr == nil || localConflictsInStatements([]statements.Statement{prevAssign}, pr) {
									conflict = true
									break
								}
							}
						}
						if !conflict {
							prevIdx := len(normalized) - 1
							normalized = append(normalized, prefix...)
							copy(normalized[prevIdx+len(prefix):], normalized[prevIdx:prevIdx+1])
							copy(normalized[prevIdx:prevIdx+len(prefix)], prefix)
							normalized = append(normalized, whileSt)
							continue
						}
					}
				}
				normalized = append(normalized, prefix...)
				normalized = append(normalized, whileSt)
				continue
			}
		}
		normalized = append(normalized, normalizedSt)
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

func extractDoWhileGuard(guard *statements.IfStatement) (values.JavaValue, []statements.Statement, bool) {
	if guard == nil || guard.Condition == nil {
		return nil, nil, false
	}
	if len(guard.IfBody) > 0 && len(guard.ElseBody) == 1 {
		if kind, label, known := sourceTransferKind(guard.ElseBody[0]); known && kind == "break" && label == "" {
			return guard.Condition, guard.IfBody, true
		}
	}
	if jdecenv.Get("JDEC_LOOP_NORM_EXT_OFF") == "" && len(guard.IfBody) == 1 && len(guard.ElseBody) > 0 {
		if kind, label, known := sourceTransferKind(guard.IfBody[0]); known && kind == "break" && label == "" {
			neg := values.SimplifyConditionValue(&values.JavaExpression{
				Op:     values.Not,
				Values: []values.JavaValue{guard.Condition},
				Typ:    types.NewJavaPrimer(types.JavaBoolean),
			})
			return neg, guard.ElseBody, true
		}
	}
	return nil, nil, false
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
	if !ok {
		return nil
	}
	cond, body, ok := extractDoWhileGuard(guard)
	if !ok {
		return nil
	}
	return &statements.WhileStatement{ConditionValue: cond, Body: trimTrailingContinue(body)}
}

func doWhileWithConstPrefixAsWhile(loop *statements.DoWhileStatement, remaining []statements.Statement) ([]statements.Statement, *statements.WhileStatement) {
	if loop == nil || loop.Label != "" || loop.ConditionValue == nil || !isTrueBooleanLiteral(loop.ConditionValue) ||
		len(loop.Body) < 2 {
		return nil, nil
	}
	last := len(loop.Body) - 1
	guard, ok := loop.Body[last].(*statements.IfStatement)
	if !ok {
		return nil, nil
	}
	cond, body, ok := extractDoWhileGuard(guard)
	if !ok {
		return nil, nil
	}
	prefix := loop.Body[:last]
	seenRefs := make([]*values.JavaRef, 0, len(prefix))
	for _, st := range prefix {
		as, ok := st.(*statements.AssignStatement)
		if !ok || as == nil || as.ArrayMember != nil || !as.IsFirst || as.IsDeclare || as.JavaValue == nil {
			return nil, nil
		}
		ref := plainLoopRef(as.LeftValue)
		if ref == nil || ref.VarUid == "" || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil {
			return nil, nil
		}
		for _, prevRef := range seenRefs {
			if sameOrAliasedLocal(prevRef, ref) {
				return nil, nil
			}
		}
		lit, ok := values.UnpackSoltValue(as.JavaValue).(*values.JavaLiteral)
		if !ok || lit == nil || !values.IsPure(as.JavaValue) {
			return nil, nil
		}
		seenRefs = append(seenRefs, ref)
		// Only hoist a per-iteration constant declaration when both the loop guard
		// (raw + normalized condition) and the loop body are proved free of writes
		// and opaque leaves, and extending the local's scope cannot collide with or
		// become observable in statements following the loop.
		if valueMayWriteOrOpaqueLocal(guard.Condition, ref) ||
			valueMayWriteOrOpaqueLocal(cond, ref) ||
			statementsMayWriteOrOpaqueLocal(body, ref) ||
			localConflictsInStatements(remaining, ref) {
			return nil, nil
		}
	}
	hoisted := append([]statements.Statement(nil), prefix...)
	return hoisted, &statements.WhileStatement{ConditionValue: cond, Body: trimTrailingContinue(body)}
}

func localRefSpelling(r *values.JavaRef) string {
	if r == nil || r.IsThis || r.CustomValue != nil || r.StackVar != nil || r.Id == nil {
		return ""
	}
	return r.String(nil)
}

func sameOrAliasedLocal(a, b *values.JavaRef) bool {
	if a == nil || b == nil {
		return false
	}
	if values.SameLocal(a, b) {
		return true
	}
	if a.Id != nil && b.Id != nil && a.Id == b.Id {
		return true
	}
	sa := localRefSpelling(a)
	return sa != "" && sa == localRefSpelling(b)
}

func isLocalWriteOperator(op string) bool {
	switch op {
	case values.INC, values.DEC, "++", "--", "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>=":
		return true
	default:
		return false
	}
}

func isSupportedWriteTargetNode(lhs values.JavaValue) (*values.JavaRef, bool) {
	switch t := lhs.(type) {
	case *values.JavaRef:
		return t, true
	case *values.RefMember, *values.JavaClassMember, *values.JavaArrayMember:
		return nil, true
	default:
		return nil, false
	}
}

func valueMayWriteOrOpaqueLocal(v values.JavaValue, target *values.JavaRef) bool {
	if v == nil {
		return false
	}
	if target == nil {
		return true
	}
	access := values.InspectAccess(v)
	if access.Effects&values.EffectOpaque != 0 {
		return true
	}
	for w := range access.Writes {
		if sameOrAliasedLocal(w, target) {
			return true
		}
	}
	seen := map[values.JavaValue]bool{}
	var walk func(values.JavaValue) bool
	walk = func(cur values.JavaValue) bool {
		if cur == nil || seen[cur] {
			return false
		}
		seen[cur] = true
		switch x := cur.(type) {
		case *values.JavaExpression:
			if x == nil {
				return false
			}
			if isLocalWriteOperator(x.Op) {
				if len(x.Values) == 0 {
					return true
				}
				ref, ok := isSupportedWriteTargetNode(values.UnpackSoltValue(x.Values[0]))
				if !ok || (ref != nil && sameOrAliasedLocal(ref, target)) {
					return true
				}
			}
		case *values.AssignmentExpression:
			if x == nil {
				return false
			}
			ref, ok := isSupportedWriteTargetNode(values.UnpackSoltValue(x.Target))
			if !ok || (ref != nil && sameOrAliasedLocal(ref, target)) {
				return true
			}
		}
		children, known := values.Children(cur)
		if !known {
			return true
		}
		for _, ch := range children {
			if walk(ch) {
				return true
			}
		}
		return false
	}
	return walk(v)
}

func valueConflictsWithLocal(v values.JavaValue, target *values.JavaRef) bool {
	if v == nil {
		return false
	}
	if target == nil {
		return true
	}
	effects, refs := values.InspectValue(v)
	if effects&values.EffectOpaque != 0 {
		return true
	}
	for r := range refs {
		if sameOrAliasedLocal(r, target) {
			return true
		}
	}
	return false
}

func statementsMayWriteOrOpaqueLocal(sts []statements.Statement, target *values.JavaRef) bool {
	return statementTreeCheckFailClosed(sts, target, valueMayWriteOrOpaqueLocal)
}

func localConflictsInStatements(sts []statements.Statement, target *values.JavaRef) bool {
	return statementTreeCheckFailClosed(sts, target, valueConflictsWithLocal)
}

// statementTreeCheckFailClosed traverses statements over the closed Statement union.
// Any unrecognized or opaque statement (e.g. uncertified CustomStatement,
// StackAssignStatement, MiddleStatement, GOTOStatement, invalid SourceAssertionStatement)
// causes the check to fail closed by returning true.
func statementTreeCheckFailClosed(sts []statements.Statement, target *values.JavaRef, checkVal func(values.JavaValue, *values.JavaRef) bool) bool {
	if len(sts) == 0 {
		return false
	}
	if target == nil {
		return true
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.AssignStatement:
			if s == nil {
				continue
			}
			if s.ArrayMember == nil {
				if lhs := plainLoopRef(s.LeftValue); lhs != nil {
					if sameOrAliasedLocal(lhs, target) {
						return true
					}
				} else {
					if _, ok := isSupportedWriteTargetNode(values.UnpackSoltValue(s.LeftValue)); !ok {
						return true
					}
					if checkVal(s.LeftValue, target) {
						return true
					}
				}
			} else if checkVal(s.ArrayMember.Object, target) || checkVal(s.ArrayMember.Index, target) {
				return true
			}
			if checkVal(s.JavaValue, target) {
				return true
			}
		case *statements.ExpressionStatement:
			if s == nil {
				continue
			}
			if checkVal(s.Expression, target) {
				return true
			}
		case *statements.ConditionStatement:
			if s == nil {
				continue
			}
			if checkVal(s.Condition, target) {
				return true
			}
		case *statements.IfStatement:
			if s == nil {
				continue
			}
			if checkVal(s.Condition, target) ||
				statementTreeCheckFailClosed(s.IfBody, target, checkVal) ||
				statementTreeCheckFailClosed(s.ElseBody, target, checkVal) {
				return true
			}
		case *statements.WhileStatement:
			if s == nil {
				continue
			}
			if checkVal(s.ConditionValue, target) || statementTreeCheckFailClosed(s.Body, target, checkVal) {
				return true
			}
		case *statements.DoWhileStatement:
			if s == nil {
				continue
			}
			if checkVal(s.ConditionValue, target) || statementTreeCheckFailClosed(s.Body, target, checkVal) {
				return true
			}
		case *statements.ForStatement:
			if s == nil {
				continue
			}
			if s.InitVar != nil && statementTreeCheckFailClosed([]statements.Statement{s.InitVar}, target, checkVal) {
				return true
			}
			if s.Condition != nil && checkVal(s.Condition.Condition, target) {
				return true
			}
			if s.EndExp != nil && statementTreeCheckFailClosed([]statements.Statement{s.EndExp}, target, checkVal) {
				return true
			}
			if statementTreeCheckFailClosed(s.SubStatements, target, checkVal) {
				return true
			}
		case *statements.SwitchStatement:
			if s == nil {
				continue
			}
			if checkVal(s.Value, target) {
				return true
			}
			for _, c := range s.Cases {
				if c != nil && statementTreeCheckFailClosed(c.Body, target, checkVal) {
					return true
				}
			}
		case *statements.TryCatchStatement:
			if s == nil {
				continue
			}
			for _, ex := range s.Exception {
				if ex != nil && sameOrAliasedLocal(ex, target) {
					return true
				}
			}
			for _, init := range s.EntryInitializers {
				if init != nil && statementTreeCheckFailClosed([]statements.Statement{init}, target, checkVal) {
					return true
				}
			}
			if statementTreeCheckFailClosed(s.TryBody, target, checkVal) {
				return true
			}
			for _, cb := range s.CatchBodies {
				if statementTreeCheckFailClosed(cb, target, checkVal) {
					return true
				}
			}
		case *statements.SynchronizedStatement:
			if s == nil {
				continue
			}
			if checkVal(s.Argument, target) || statementTreeCheckFailClosed(s.Body, target, checkVal) {
				return true
			}
		case *statements.ReturnStatement:
			if s == nil {
				continue
			}
			if checkVal(s.JavaValue, target) {
				return true
			}
		case *statements.SourceAssertionStatement:
			if s == nil {
				return true
			}
			cond, call, _, valid := s.SourceAssertionProtocol()
			if !valid || checkVal(cond, target) || checkVal(call, target) {
				return true
			}
		case *statements.SourceAnchorStatement, *statements.NewStatement:
			continue
		case *statements.CustomStatement:
			if s == nil {
				continue
			}
			if as, ok := s.SourceAssignmentForward(); ok && as != nil {
				if statementTreeCheckFailClosed([]statements.Statement{as}, target, checkVal) {
					return true
				}
				continue
			}
			if op, ok := s.SourceThrowOperand(); ok {
				if checkVal(op, target) {
					return true
				}
				continue
			}
			if _, _, known := s.SourceTransferProtocol(); known {
				continue
			}
			return true
		default:
			return true
		}
	}
	return false
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
	if assign, ok := st.(*statements.AssignStatement); ok {
		if assign == nil || assign.IsFirst || assign.IsDeclare || assign.ArrayMember != nil || jdecenv.Get("JDEC_LOOP_NORM_EXT_OFF") != "" {
			return false
		}
		lhs := plainLoopRef(assign.LeftValue)
		if lhs == nil || lhs.VarUid != uid {
			return false
		}
		es, ok := values.UnpackSoltValue(assign.JavaValue).(*values.JavaExpression)
		if !ok || es == nil || (es.Op != values.ADD && es.Op != values.SUB) || len(es.Values) != 2 || !values.IsPure(es) {
			return false
		}
		first := plainLoopRef(es.Values[0])
		return first != nil && first.VarUid == uid
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
