package rewriter

import "github.com/yaklang/javajive/classparser/decompiler/core/statements"

// FinallyConsumesSharedVoidReturn proves that structuring has already copied
// the same decoded, effect-free RETURN into every normal try/catch completion.
// The original shared node still follows the region in the IR, but Java would
// reject that duplicate as unreachable. Only suppress its rendering after the
// complete original finally-domain/effect proof succeeds. Retain the IR and all
// real evaluations; unknown, value-bearing or different-PC exits fail closed.
func FinallyConsumesSharedVoidReturn(tr *statements.TryCatchStatement, tail *statements.ReturnStatement) bool {
	if tail == nil || tail.JavaValue != nil || !tail.HasOriginPC || tail.OriginPC < 0 {
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
			return last != nil && last.JavaValue == nil && last.HasOriginPC && last.OriginPC == tail.OriginPC
		case *statements.IfStatement:
			return last != nil && last.Condition != nil && returns(last.IfBody, depth+1) && returns(last.ElseBody, depth+1)
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
