package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Reaching-definition webs need not overlap when a nonthrowing flag STORE is
// the last protected instruction. Its normal guard reads the new definition;
// the handler guard reads the earlier definition. For a source finally these
// two definitions must name the same original physical boolean location.
// Rebind only a private source view, after reopening both STORE witnesses and
// excluding other uses. No initializer, effect, handler, or canonical AST moves.
func finallyFlagSourceView(tr *statements.TryCatchStatement) (*statements.TryCatchStatement, bool) {
	if tr == nil || len(tr.Exception) == 0 || len(tr.Handlers) != len(tr.Exception) || len(tr.CatchBodies) != len(tr.Exception) || len(tr.TryBody) > 128 {
		return nil, false
	}
	last := len(tr.Handlers) - 1
	h := tr.Handlers[last]
	if !h.CatchAll || len(h.ProtectedRanges) == 0 || len(h.ProtectedRanges) > 32 {
		return nil, false
	}
	handler := finallyWithoutEnd(tr.CatchBodies[last])
	if len(handler) != 2 {
		return nil, false
	}
	exceptional, guarded := handler[0].(*statements.IfStatement)
	if !guarded || exceptional == nil || len(exceptional.ElseBody) != 0 {
		return nil, false
	}
	outer, exceptionalNot, local := finallyFlagCondition(exceptional.Condition, 0)
	boolean := func(ref *values.JavaRef) bool {
		if ref == nil || ref.Id == nil || ref.VarUid == "" || ref.IsThis || ref.IsParam || !finallyPureLocal(ref) || ref.Type() == nil {
			return false
		}
		kind, known := ref.Type().RawType().(*types.JavaPrimer)
		return known && kind != nil && kind.Name == types.JavaBoolean
	}
	if !local || !boolean(outer) || !outer.SameOriginalLocalDeclaration(outer) {
		return nil, false
	}
	initialPC, slot, original := outer.OriginalLocalDeclaration(outer.Val)
	if !original {
		return nil, false
	}
	for _, row := range h.ProtectedRanges {
		if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 || initialPC >= row[0] {
			return nil, false
		}
	}
	body := finallyWithoutEnd(tr.TryBody)
	end := len(body)
	if end > 0 {
		if _, returning := body[end-1].(*statements.ReturnStatement); returning {
			end--
		}
	}
	if end < 2 {
		return nil, false
	}
	store, assigned := body[end-2].(*statements.AssignStatement)
	normal, guarded := body[end-1].(*statements.IfStatement)
	if !assigned || !guarded || store == nil || normal == nil || !store.IsFirst || store.IsDeclare || store.ArrayMember != nil || len(normal.ElseBody) != 0 {
		return nil, false
	}
	inner, local := plainTryValue(store.LeftValue).(*values.JavaRef)
	normalRef, normalNot, normalKnown := finallyFlagCondition(normal.Condition, 0)
	pc, storedSlot, witnessed := store.OriginalLocalStore()
	if !local || !boolean(inner) || sameTryLocal(inner, outer) || !normalKnown || normalNot != exceptionalNot || !sameTryLocal(normalRef, inner) || !witnessed || storedSlot != slot || pc <= initialPC || !finallyContains(h.ProtectedRanges, pc) {
		return nil, false
	}
	definitionPC, definitionSlot, known := inner.OriginalLocalDeclaration(store.JavaValue)
	if !known || definitionPC != pc || definitionSlot != slot || !sameFinallyStableValue(store.JavaValue, store.JavaValue, nil, 0) {
		return nil, false
	}
	// The new source binder is mutable, but no prior lambda/callback or sibling
	// source expression may have captured either split declaration.
	bindings := map[*utils.VariableId]int{outer.Id: 1, inner.Id: 2}
	if !finallyCleanupBindingsScoped(body[:end-2], nil, bindings) || !finallyCleanupBindingsScoped(normal.IfBody, nil, bindings) || !finallyCleanupBindingsScoped(exceptional.IfBody, nil, bindings) || !finallyCleanupBindingsScoped(body[end:], nil, bindings) {
		return nil, false
	}
	clone, assignment, guard := *tr, *store, *normal
	assignment.LeftValue, assignment.IsFirst, assignment.Final = outer, false, false
	condition := values.JavaValue(outer)
	if normalNot {
		condition = &values.JavaExpression{Op: values.Not, Values: []values.JavaValue{outer}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
	}
	guard.Condition = condition
	clone.TryBody = append([]statements.Statement(nil), tr.TryBody...)
	clone.TryBody[end-2], clone.TryBody[end-1] = &assignment, &guard
	// IFEQ/IFNE can retain a typed ==0/!=0 expression on just one path.
	// Normalize both pure reads in this view; no rendered text is compared.
	handlerGuard := *exceptional
	handlerGuard.Condition = condition
	clone.CatchBodies = append([][]statements.Statement(nil), tr.CatchBodies...)
	clone.CatchBodies[last] = append([]statements.Statement(nil), tr.CatchBodies[last]...)
	clone.CatchBodies[last][0] = &handlerGuard
	return &clone, true
}

func finallyFlagCondition(condition values.JavaValue, depth int) (*values.JavaRef, bool, bool) {
	if depth > 8 {
		return nil, false, false
	}
	condition = plainTryValue(condition)
	if ref, local := condition.(*values.JavaRef); local && ref != nil && ref.Type() != nil {
		kind, known := ref.Type().RawType().(*types.JavaPrimer)
		return ref, false, known && kind != nil && kind.Name == types.JavaBoolean && finallyPureLocal(ref)
	}
	expr, expression := condition.(*values.JavaExpression)
	if !expression || expr == nil || expr.Typ == nil {
		return nil, false, false
	}
	kind, known := expr.Typ.RawType().(*types.JavaPrimer)
	if !known || kind == nil || kind.Name != types.JavaBoolean {
		return nil, false, false
	}
	if expr.Op == values.Not && len(expr.Values) == 1 {
		ref, negative, known := finallyFlagCondition(expr.Values[0], depth+1)
		return ref, !negative, known
	}
	if (expr.Op != values.EQ && expr.Op != values.NEQ) || len(expr.Values) != 2 {
		return nil, false, false
	}
	for i, value := range expr.Values {
		literal, constant := plainTryValue(value).(*values.JavaLiteral)
		if !constant || literal == nil || literal.Type() == nil {
			continue
		}
		wordType, known := literal.Type().RawType().(*types.JavaPrimer)
		if !known || wordType == nil || wordType.Name != types.JavaBoolean && wordType.Name != types.JavaInteger {
			continue
		}
		one, zero := false, false
		switch word := literal.Data.(type) {
		case bool:
			one, zero = word, !word
		case int:
			one, zero = word == 1, word == 0
		case int32:
			one, zero = word == 1, word == 0
		}
		if !one && !zero {
			continue
		}
		ref, negative, known := finallyFlagCondition(expr.Values[1-i], depth+1)
		if expr.Op == values.EQ && zero || expr.Op == values.NEQ && one {
			negative = !negative
		}
		return ref, negative, known
	}
	return nil, false, false
}
