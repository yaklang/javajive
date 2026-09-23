package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// InlineGuardedCallTemps moves a single-use instance call into the non-null arm of an
// immediately following null-guarded ternary. It repairs the bytecode shape
// `tmp = receiver.call(); result = receiver == null ? null : tmp` only when the
// receiver comparison is pure, no other statement uses tmp, and exception-handler
// coverage is identical. The call then runs only on the same path as in the bytecode.
func (d *Decompiler) InlineGuardedCallTemps(root *[]statements.Statement) int {
	if d == nil || root == nil {
		return 0
	}
	if d.Env != nil && d.Env("JDEC_GUARDED_CALL_INLINE_OFF") != "" {
		return 0
	}
	changed := 0
	var rewriteList func(*[]statements.Statement)
	rewriteList = func(list *[]statements.Statement) {
		if list == nil {
			return
		}
		for _, statement := range *list {
			switch s := statement.(type) {
			case *statements.IfStatement:
				rewriteList(&s.IfBody)
				rewriteList(&s.ElseBody)
			case *statements.ForStatement:
				rewriteList(&s.SubStatements)
			case *statements.WhileStatement:
				rewriteList(&s.Body)
			case *statements.DoWhileStatement:
				rewriteList(&s.Body)
			case *statements.SwitchStatement:
				for _, item := range s.Cases {
					rewriteList(&item.Body)
				}
			case *statements.TryCatchStatement:
				rewriteList(&s.TryBody)
				for i := range s.CatchBodies {
					rewriteList(&s.CatchBodies[i])
				}
			case *statements.SynchronizedStatement:
				rewriteList(&s.Body)
			}
		}

		for i := 0; i+1 < len(*list); {
			items := *list
			producer, ok := items[i].(*statements.AssignStatement)
			if !ok || i+1 >= len(items) {
				i++
				continue
			}
			consumer, ok := items[i+1].(*statements.AssignStatement)
			if !ok || !d.canInlineGuardedCallTemp(*root, producer, consumer) {
				i++
				continue
			}
			temp := values.UnpackSoltValue(producer.LeftValue).(*values.JavaRef)
			ternary := values.UnpackSoltValue(consumer.JavaValue).(*values.TernaryExpression)
			_, nullWhenTrue, _ := nonNullGuard(ternary.Condition)
			var replaced bool
			if nullWhenTrue {
				ternary.FalseValue, replaced = replaceGuardedTemp(ternary.FalseValue, temp, producer.JavaValue)
			} else {
				ternary.TrueValue, replaced = replaceGuardedTemp(ternary.TrueValue, temp, producer.JavaValue)
			}
			if !replaced {
				i++
				continue
			}
			copy(items[i:], items[i+1:])
			items[len(items)-1] = nil
			*list = items[:len(items)-1]
			changed++
		}
	}
	rewriteList(root)
	return changed
}

func (d *Decompiler) canInlineGuardedCallTemp(root []statements.Statement, producer, consumer *statements.AssignStatement) bool {
	if producer == nil || consumer == nil || producer.ArrayMember != nil || consumer.ArrayMember != nil ||
		!producer.HasOriginPC || !consumer.HasOriginPC ||
		producer.JavaValue == nil || consumer.JavaValue == nil {
		return false
	}
	temp, ok := values.UnpackSoltValue(producer.LeftValue).(*values.JavaRef)
	if !ok || temp == nil {
		return false
	}
	ternary, ok := values.UnpackSoltValue(consumer.JavaValue).(*values.TernaryExpression)
	if !ok || ternary == nil {
		return false
	}
	call := guardedInstanceCall(producer.JavaValue)
	if call == nil || call.FunctionName == "<init>" || call.IsSpecialInvoke || call.Descriptor == "" {
		return false
	}
	receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
	if !ok || receiver == nil || values.SameLocal(receiver, temp) {
		return false
	}
	guard, nullWhenTrue, ok := nonNullGuard(ternary.Condition)
	if !ok || !values.SameLocal(receiver, guard) {
		return false
	}
	trueTemp, trueIsTemp := guardedTempRef(ternary.TrueValue)
	falseTemp, falseIsTemp := guardedTempRef(ternary.FalseValue)
	trueNull := isJavaNull(ternary.TrueValue)
	falseNull := isJavaNull(ternary.FalseValue)
	if nullWhenTrue {
		if !falseIsTemp || !values.SameLocal(falseTemp, temp) || !trueNull || trueIsTemp {
			return false
		}
	} else if !trueIsTemp || !values.SameLocal(trueTemp, temp) || !falseNull || falseIsTemp {
		return false
	}
	target, ok := values.UnpackSoltValue(consumer.LeftValue).(*values.JavaRef)
	if !ok || target == nil || values.SameLocal(target, temp) {
		return false
	}
	if statementTreeReferencesLocal(root, temp, producer, consumer) {
		return false
	}

	operation := values.InspectAccess(producer.JavaValue)
	if operation.Effects&values.EffectCall == 0 || operation.Effects&values.EffectOpaque != 0 {
		return false
	}
	conditionPC, ok := d.opcodeIndexToOffset[ternary.ConditionFromOp]
	if !ok || call.OriginPC <= int(conditionPC) {
		// The original branch must precede the invoke. Otherwise this may be a real
		// eager call followed by a null test, whose exception behavior must be kept.
		return false
	}
	operation.Handlers = d.handlersAtPC(call.OriginPC)
	prefix := values.InspectAccess(ternary.Condition)
	if prefix.Effects != 0 {
		return false
	}
	prefix.Handlers = d.handlersAtPC(int(conditionPC))
	return values.CanMoveAcrossPrefix(operation, prefix)
}

func (d *Decompiler) handlersAtPC(pc int) []int {
	var handlers []int
	for i, handler := range d.ExceptionTable {
		if pc >= int(handler.StartPc) && pc < int(handler.EndPc) {
			handlers = append(handlers, i)
		}
	}
	return handlers
}

func guardedInstanceCall(value values.JavaValue) *values.FunctionCallExpression {
	for value != nil {
		switch v := values.UnpackSoltValue(value).(type) {
		case *values.FunctionCallExpression:
			return v
		case *values.CastExpression:
			value = v.Value
		case *values.EffectTag:
			value = v.Inner
		default:
			return nil
		}
	}
	return nil
}

func nonNullGuard(value values.JavaValue) (ref *values.JavaRef, nullWhenTrue bool, ok bool) {
	condition, ok := values.UnpackSoltValue(value).(*values.JavaExpression)
	if !ok || condition == nil || len(condition.Values) != 2 ||
		(condition.Op != values.EQ && condition.Op != values.NEQ) {
		return nil, false, false
	}
	left := values.UnpackSoltValue(condition.Values[0])
	right := values.UnpackSoltValue(condition.Values[1])
	if r, isRef := left.(*values.JavaRef); isRef && isJavaNull(right) {
		return r, condition.Op == values.EQ, true
	}
	if r, isRef := right.(*values.JavaRef); isRef && isJavaNull(left) {
		return r, condition.Op == values.EQ, true
	}
	return nil, false, false
}

func isJavaNull(value values.JavaValue) bool {
	return value == values.JavaNull || values.IsNullLiteral(values.UnpackSoltValue(value))
}

func guardedTempRef(value values.JavaValue) (*values.JavaRef, bool) {
	for value != nil {
		switch v := values.UnpackSoltValue(value).(type) {
		case *values.JavaRef:
			return v, true
		case *values.CastExpression:
			value = v.Value
		default:
			return nil, false
		}
	}
	return nil, false
}

func replaceGuardedTemp(value values.JavaValue, temp *values.JavaRef, replacement values.JavaValue) (values.JavaValue, bool) {
	value = values.UnpackSoltValue(value)
	switch v := value.(type) {
	case *values.JavaRef:
		if values.SameLocal(v, temp) {
			return replacement, true
		}
	case *values.CastExpression:
		copy := *v
		var replaced bool
		copy.Value, replaced = replaceGuardedTemp(v.Value, temp, replacement)
		if !replaced {
			return nil, false
		}
		return &copy, true
	}
	return nil, false
}

func statementTreeReferencesLocal(list []statements.Statement, ref *values.JavaRef, skip ...statements.Statement) bool {
	for _, statement := range list {
		if statement == nil || statementIsSkipped(statement, skip) {
			continue
		}
		if statementReferencesLocal(statement, ref, skip...) {
			return true
		}
	}
	return false
}

func statementIsSkipped(statement statements.Statement, skip []statements.Statement) bool {
	for _, candidate := range skip {
		if statement == candidate {
			return true
		}
	}
	return false
}

func statementReferencesLocal(statement statements.Statement, ref *values.JavaRef, skip ...statements.Statement) bool {
	if statement == nil {
		return false
	}
	read := func(value values.JavaValue) bool {
		effects, refs := values.InspectValue(value)
		if effects&values.EffectOpaque != 0 {
			// Unknown source closures may hide a read of this local. Do not delete
			// its defining assignment when the use tree cannot be enumerated.
			return true
		}
		for candidate := range refs {
			if values.SameLocal(candidate, ref) {
				return true
			}
		}
		return false
	}
	listReads := func(list []statements.Statement) bool { return statementTreeReferencesLocal(list, ref, skip...) }
	switch s := statement.(type) {
	case *statements.AssignStatement:
		if left, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok && values.SameLocal(left, ref) {
			return true
		}
		return read(s.JavaValue) || read(s.ArrayMember)
	case *statements.StackAssignStatement:
		return read(s.JavaValue)
	case *statements.ReturnStatement:
		return read(s.JavaValue)
	case *statements.ExpressionStatement:
		return read(s.Expression)
	case *statements.ConditionStatement:
		return read(s.Condition)
	case *values.JavaExpression:
		return read(s)
	case *statements.IfStatement:
		return read(s.Condition) || listReads(s.IfBody) || listReads(s.ElseBody)
	case *statements.ForStatement:
		return statementReferencesLocal(s.InitVar, ref, skip...) || statementReferencesLocal(s.Condition, ref, skip...) ||
			statementReferencesLocal(s.EndExp, ref, skip...) || listReads(s.SubStatements)
	case *statements.WhileStatement:
		return read(s.ConditionValue) || listReads(s.Body)
	case *statements.DoWhileStatement:
		return read(s.ConditionValue) || listReads(s.Body)
	case *statements.SwitchStatement:
		if read(s.Value) {
			return true
		}
		for _, item := range s.Cases {
			if item != nil && listReads(item.Body) {
				return true
			}
		}
		return false
	case *statements.TryCatchStatement:
		for _, exception := range s.Exception {
			if values.SameLocal(exception, ref) {
				return true
			}
		}
		if listReads(s.TryBody) {
			return true
		}
		for _, body := range s.CatchBodies {
			if listReads(body) {
				return true
			}
		}
		return false
	case *statements.SynchronizedStatement:
		return read(s.Argument) || listReads(s.Body)
	case *statements.CustomStatement:
		// Custom source fragments have no typed child visitor. Only structured control
		// transfers are known to contain no local reads; every other fragment is opaque.
		text := strings.TrimSpace(s.String(&class_context.ClassContext{}))
		if text == "break" || text == "continue" || strings.HasPrefix(text, "break ") || strings.HasPrefix(text, "continue ") {
			return false
		}
		return true
	case *statements.GOTOStatement, *statements.NewStatement, *statements.MiddleStatement:
		return false
	default:
		return true
	}
}
