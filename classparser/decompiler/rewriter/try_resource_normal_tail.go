package rewriter

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A resource's successful close is outside the body handler. When branch
// structuring nests close+return inside the try, a close failure is caught as a
// body failure and invokes the close again. Prove the normal tail from decoded
// PCs and the exceptional cleanup's identical invoke tuple, then factor that
// tail across every successful arm. Abrupt arms retain their original effects.
func factorResourceNormalTail(region *core.Node, tr *statements.TryCatchStatement) (*statements.AssignStatement, []statements.Statement) {
	if region == nil || tr == nil || len(tr.Exception) != 1 || len(tr.Handlers) != 1 || len(tr.CatchBodies) != 1 || tr.Handlers[0].CatchAll {
		return nil, nil
	}
	if region.SharedProtectedHandler && !resourceSharedNestedHandlerRanges(region, tr) {
		return nil, nil
	}
	outer := tr.Exception[0]
	if outer == nil || !resourceThrowableType(outer.Type()) {
		return nil, nil
	}
	rows := tr.Handlers[0].ProtectedRanges
	if len(rows) == 0 || len(rows) > 32 {
		return nil, nil
	}
	covered := func(pc int) bool { return finallyContains(rows, pc) }
	handler := finallyWithoutEnd(tr.CatchBodies[0])
	if len(handler) != 2 {
		return nil, nil
	}
	rethrow, ok := handler[1].(*statements.CustomStatement)
	if !ok || rethrow == nil || !sameTryLocal(rethrow.ThrownValue, outer) {
		return nil, nil
	}
	resource, exceptionalClose, ok := resourceTailExceptionalClose(handler[0], outer)
	if !ok {
		return nil, nil
	}
	rawRows := make([]core.HandlerRange, 0, len(rows))
	for _, row := range rows {
		if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 {
			return nil, nil
		}
		rawRows = append(rawRows, core.HandlerRange{StartPc: uint16(row[0]), EndPc: uint16(row[1])})
	}
	var result *values.JavaRef
	var normalClose statements.Statement
	var normalGuarded bool
	returnMode := 0 // unknown, void, value
	var checkedPrefix func([]statements.Statement) ([]statements.Statement, bool)
	remaining := 128
	var walk func([]statements.Statement) ([]statements.Statement, bool)
	walk = func(input []statements.Statement) ([]statements.Statement, bool) {
		remaining--
		if remaining < 0 {
			return nil, false
		}
		body := finallyWithoutEnd(input)
		if len(body) == 0 {
			return nil, false
		}
		end := body[len(body)-1]
		switch last := end.(type) {
		case *statements.ReturnStatement:
			if last == nil || !last.HasOriginPC || covered(last.OriginPC) || (last.JavaValue != nil && (!finallyPureLocal(last.JavaValue) || last.JavaValue.Type() == nil)) || len(body) < 2 {
				return nil, false
			}
			closeStatement := body[len(body)-2]
			closeResource, normalCall, guarded, ok := resourceTailNormalClose(closeStatement)
			if !ok || !sameTryLocal(closeResource, resource) || !sameUnprotectedTryCall(rawRows, normalCall, exceptionalClose) {
				return nil, false
			}
			if normalClose != nil && normalGuarded != guarded {
				return nil, false
			}
			mode := 1
			if last.JavaValue != nil {
				mode = 2
			}
			if returnMode != 0 && returnMode != mode {
				return nil, false
			}
			returnMode = mode
			if normalClose == nil {
				normalClose = closeStatement
				normalGuarded = guarded
			}
			if mode == 2 && result == nil {
				result = values.NewJavaRef(utils.NewRootVariableId(), nil, last.JavaValue.Type().Copy())
				normalClose = closeStatement
			}
			if mode == 2 && !reflect.DeepEqual(last.JavaValue.Type().RawType(), result.Type().RawType()) {
				return nil, false
			}
			prefix := body[:len(body)-2]
			checked, ok := checkedPrefix(prefix)
			if !ok {
				return nil, false
			}
			if mode == 1 {
				return checked, true
			}
			return append(checked, statements.NewAssignStatement(result, last.JavaValue, false)), true
		case *statements.CustomStatement:
			if last == nil || last.ThrownValue == nil || !last.HasOriginPC || !covered(last.OriginPC) || !finallyCoveredValue(last.ThrownValue, covered) {
				return nil, false
			}
			proof := finallyProof{remaining: 256}
			checked, _, ok := proof.block(body, true, covered)
			return checked, ok
		case *statements.IfStatement:
			if last == nil || !finallyCoveredValue(last.Condition, covered) {
				return nil, false
			}
			clone := *last
			var a, b bool
			clone.IfBody, a = walk(last.IfBody)
			clone.ElseBody, b = walk(last.ElseBody)
			if !a || !b {
				return nil, false
			}
			checked, ok := checkedPrefix(body[:len(body)-1])
			if !ok {
				return nil, false
			}
			return append(checked, &clone), true
		case *statements.TryCatchStatement:
			// A guarded close can leave the complete normal exit inside a
			// structured inner try. Since this is the final source statement,
			// every normal/caught arm may share the outer unprotected tail,
			// but only after each arm proves its own close+return or throw.
			if !resourceNestedHandlersCovered(last, rows) {
				return nil, false
			}
			checked, ok := checkedPrefix(body[:len(body)-1])
			if !ok {
				return nil, false
			}
			clone := *last
			clone.TryBody, ok = walk(last.TryBody)
			if !ok {
				return nil, false
			}
			clone.CatchBodies = make([][]statements.Statement, len(last.CatchBodies))
			for i, caught := range last.CatchBodies {
				clone.CatchBodies[i], ok = walk(caught)
				if !ok {
					return nil, false
				}
			}
			return append(checked, &clone), true
		default:
			return nil, false
		}
	}
	// A terminal nested try may return from one caught arm while its normal
	// arm falls through to this region's close+return. All captured prefixes
	// remain protected; stripping those early void exits is safe only with no
	// protected continuation after the nested try.
	checkedPrefix = func(prefix []statements.Statement) ([]statements.Statement, bool) {
		remaining--
		if remaining < 0 {
			return nil, false
		}
		proof := finallyProof{remaining: 256}
		checked, exits, ok := proof.block(prefix, false, covered)
		if ok && !exits {
			return checked, true
		}
		if len(prefix) == 0 {
			return nil, false
		}
		nested, ok := prefix[len(prefix)-1].(*statements.TryCatchStatement)
		if !ok || !resourceNestedHandlersCovered(nested, rows) {
			return nil, false
		}
		before := finallyProof{remaining: 256}
		head, ends, valid := before.block(prefix[:len(prefix)-1], false, covered)
		if !valid || ends {
			return nil, false
		}
		inner := finallyProof{remaining: 256}
		normal, ends, valid := inner.block(nested.TryBody, false, covered)
		if !valid || ends {
			return nil, false
		}
		clone := *nested
		clone.TryBody = normal
		clone.CatchBodies = make([][]statements.Statement, len(nested.CatchBodies))
		for i := range nested.Handlers {
			clone.CatchBodies[i], valid = walk(nested.CatchBodies[i])
			if !valid || returnMode != 1 {
				return nil, false
			}
		}
		return append(head, &clone), true
	}
	body, ok := walk(tr.TryBody)
	if !ok || returnMode == 0 || normalClose == nil {
		return nil, nil
	}
	// The only mutations occur after every successful / abrupt arm was proved.
	tr.TryBody = body
	if returnMode == 1 {
		return nil, []statements.Statement{normalClose, statements.NewReturnStatement(nil)}
	}
	return &statements.AssignStatement{LeftValue: result, IsDeclare: true}, []statements.Statement{normalClose, statements.NewReturnStatement(result)}
}

func resourceNestedHandlersCovered(nested *statements.TryCatchStatement, rows [][2]int) bool {
	if nested == nil || len(nested.Exception) == 0 || len(nested.Exception) > 16 || len(nested.Exception) != len(nested.Handlers) || len(nested.Exception) != len(nested.CatchBodies) {
		return false
	}
	for i, h := range nested.Handlers {
		ranges, valid := canonicalHandlerRanges(h.ProtectedRanges)
		if nested.Exception[i] == nil || !finallyContains(rows, h.EntryPC) || !valid {
			return false
		}
		for _, r := range ranges {
			if !handlerIntervalCovered(rows, r) {
				return false
			}
		}
	}
	return true
}

// Direct compiler cleanup stays direct: this proof never invents a null guard.
// Guard parity is checked across every successful exit before coalescing them.
// Keep the older nullable-finally matcher's guard-only helpers unchanged.
func resourceTailNormalClose(st statements.Statement) (*values.JavaRef, *values.FunctionCallExpression, bool, bool) {
	if call, ok := resourceVoidCall(st); ok {
		resource, known := plainTryValue(call.Object).(*values.JavaRef)
		return resource, call, false, known && resource != nil
	}
	resource, call, ok := resourceNormalClose(st)
	return resource, call, true, ok
}

func resourceTailExceptionalClose(st statements.Statement, primary *values.JavaRef) (*values.JavaRef, *values.FunctionCallExpression, bool) {
	if inner, ok := st.(*statements.TryCatchStatement); ok {
		call, known := resourceSuppressedClose(inner, primary)
		if !known {
			return nil, nil, false
		}
		resource, known := plainTryValue(call.Object).(*values.JavaRef)
		return resource, call, known && resource != nil
	}
	return resourceExceptionalClose(st, primary)
}

func resourceThrowableType(t types.JavaType) bool {
	if t == nil {
		return false
	}
	name, ok := types.RawClassFQN(t)
	return ok && (strings.ReplaceAll(name, "/", ".") == "java.lang.Throwable" || name == "Throwable")
}

func resourceCloseGuard(st statements.Statement) (*values.JavaRef, []statements.Statement, bool) {
	branch, ok := st.(*statements.IfStatement)
	if !ok || branch == nil || len(finallyWithoutEnd(branch.ElseBody)) != 0 {
		return nil, nil, false
	}
	condition, ok := plainTryValue(branch.Condition).(*values.JavaExpression)
	if !ok || condition == nil || condition.Op != values.NEQ || len(condition.Values) != 2 {
		return nil, nil, false
	}
	var ref *values.JavaRef
	if condition.Values[1] == values.JavaNull || values.IsNullLiteral(condition.Values[1]) {
		ref, _ = plainTryValue(condition.Values[0]).(*values.JavaRef)
	} else if condition.Values[0] == values.JavaNull || values.IsNullLiteral(condition.Values[0]) {
		ref, _ = plainTryValue(condition.Values[1]).(*values.JavaRef)
	}
	if ref == nil || ref.Id == nil || ref.CustomValue != nil || ref.StackVar != nil {
		return nil, nil, false
	}
	return ref, finallyWithoutEnd(branch.IfBody), true
}

func resourceVoidCall(st statements.Statement) (*values.FunctionCallExpression, bool) {
	expression, ok := st.(*statements.ExpressionStatement)
	if !ok || expression == nil {
		return nil, false
	}
	call, ok := plainTryValue(expression.Expression).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC || call.IsStatic || call.Kind != values.InvokeVirtual && call.Kind != values.InvokeInterface || call.Descriptor != "()V" || call.ClassName == "" || call.FunctionName == "" || len(call.Arguments) != 0 {
		return nil, false
	}
	if _, ok := plainTryValue(call.Object).(*values.JavaRef); !ok {
		return nil, false
	}
	return call, true
}

func resourceNormalClose(st statements.Statement) (*values.JavaRef, *values.FunctionCallExpression, bool) {
	resource, body, ok := resourceCloseGuard(st)
	if !ok || len(body) != 1 {
		return nil, nil, false
	}
	call, ok := resourceVoidCall(body[0])
	if !ok || !sameTryLocal(call.Object, resource) {
		return nil, nil, false
	}
	return resource, call, true
}

func resourceExceptionalClose(st statements.Statement, primary *values.JavaRef) (*values.JavaRef, *values.FunctionCallExpression, bool) {
	resource, body, ok := resourceCloseGuard(st)
	if !ok || len(body) != 1 {
		return nil, nil, false
	}
	inner, ok := body[0].(*statements.TryCatchStatement)
	if !ok {
		return nil, nil, false
	}
	call, ok := resourceSuppressedClose(inner, primary)
	if !ok || !sameTryLocal(call.Object, resource) {
		return nil, nil, false
	}
	return resource, call, true
}

func resourceSuppressedClose(inner *statements.TryCatchStatement, primary *values.JavaRef) (*values.FunctionCallExpression, bool) {
	if inner == nil || len(inner.TryBody) != 1 || len(inner.Exception) != 1 || len(inner.Handlers) != 1 || len(inner.CatchBodies) != 1 || inner.Exception[0] == nil || !resourceThrowableType(inner.Exception[0].Type()) {
		return nil, false
	}
	call, ok := resourceVoidCall(inner.TryBody[0])
	if !ok || !finallyContains(inner.Handlers[0].ProtectedRanges, call.OriginPC) {
		return nil, false
	}
	caught := finallyWithoutEnd(inner.CatchBodies[0])
	if len(caught) != 1 {
		return nil, false
	}
	expression, ok := caught[0].(*statements.ExpressionStatement)
	if !ok || expression == nil {
		return nil, false
	}
	sink, ok := plainTryValue(expression.Expression).(*values.FunctionCallExpression)
	if !ok || sink == nil || !sink.HasOriginPC || !sameTryLocal(sink.Object, primary) || sink.Kind != values.InvokeVirtual || sink.Descriptor != "(Ljava/lang/Throwable;)V" || sink.ClassName == "" || sink.FunctionName == "" || len(sink.Arguments) != 1 || !sameTryLocal(sink.Arguments[0], inner.Exception[0]) {
		return nil, false
	}
	return call, true
}
