package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Follow only private, decoded assignments immediately preceding the region.
// Any control transfer or opaque effect ends the entry-state proof.
func resourceEntryInitializers(region *core.Node) []*statements.AssignStatement {
	var out []*statements.AssignStatement
	seen := map[*core.Node]bool{}
	for i := 0; i < 32 && region != nil && len(region.Source) == 1; i++ {
		prior := region.Source[0]
		if prior == nil || seen[prior] || len(prior.Next) != 1 || prior.Next[0] != region {
			break
		}
		seen[prior] = true
		assignment, ok := prior.Statement.(*statements.AssignStatement)
		if !ok || assignment == nil || assignment.ArrayMember != nil || !assignment.HasOriginPC || !finallyPureLocal(assignment.LeftValue) {
			break
		}
		out = append(out, assignment)
		region = prior
	}
	return out
}

// Older javac resource lowering captures a primary throwable, then protects
// that handler with a distinct catch-all. A finally view preserves both normal
// close and suppressed-exception semantics only with original region, state,
// operand and invoke witnesses; it never creates a wrapper throwable.
func recoverPrimaryResourceFinally(tr *statements.TryCatchStatement) (*FinallyRegion, bool) {
	if tr == nil || len(tr.Exception) != 2 || len(tr.Handlers) != 2 || len(tr.CatchBodies) != 2 || tr.Exception[0] == nil || tr.Exception[1] == nil || !resourceThrowableType(tr.Exception[0].Type()) || tr.Handlers[0].CatchAll || !tr.Handlers[1].CatchAll {
		return nil, false
	}
	outer := tr.Handlers[1]
	typed := tr.Handlers[0]
	if len(outer.ProtectedRanges) == 0 || len(typed.ProtectedRanges) == 0 || !finallyContains(outer.ProtectedRanges, typed.EntryPC) {
		return nil, false
	}
	var rows []core.HandlerRange
	for _, h := range tr.Handlers {
		for _, r := range h.ProtectedRanges {
			if r[0] < 0 || r[0] >= r[1] || r[1] > 65535 {
				return nil, false
			}
			rows = append(rows, core.HandlerRange{StartPc: uint16(r[0]), EndPc: uint16(r[1])})
		}
	}
	caughtBody := finallyWithoutEnd(tr.CatchBodies[0])
	if len(caughtBody) != 2 && len(caughtBody) != 3 {
		return nil, false
	}
	capture, ok := caughtBody[len(caughtBody)-2].(*statements.AssignStatement)
	if !ok || capture == nil || capture.ArrayMember != nil || !capture.HasOriginPC || !finallyContains(outer.ProtectedRanges, capture.OriginPC) || !sameTryLocal(capture.JavaValue, tr.Exception[0]) {
		return nil, false
	}
	primary, ok := plainTryValue(capture.LeftValue).(*values.JavaRef)
	if !ok || primary == nil || primary.Id == nil || primary.CustomValue != nil || primary.StackVar != nil || sameTryLocal(primary, tr.Exception[0]) {
		return nil, false
	}
	thrown, ok := caughtBody[len(caughtBody)-1].(*statements.CustomStatement)
	if !ok || thrown == nil || !thrown.HasOriginPC || !finallyContains(outer.ProtectedRanges, thrown.OriginPC) {
		return nil, false
	}
	if len(caughtBody) == 3 {
		alias, ok := caughtBody[0].(*statements.AssignStatement)
		if !ok || alias == nil || alias.ArrayMember != nil || !alias.HasOriginPC || !finallyContains(outer.ProtectedRanges, alias.OriginPC) || !sameTryLocal(alias.JavaValue, tr.Exception[0]) || !finallyPureLocal(alias.LeftValue) || !sameTryLocal(thrown.ThrownValue, plainResourceRef(alias.LeftValue)) {
			return nil, false
		}
	} else if !sameTryLocal(thrown.ThrownValue, tr.Exception[0]) {
		return nil, false
	}
	initialized := false
	for _, as := range tr.EntryInitializers {
		if as == nil {
			return nil, false
		}
		if sameTryLocal(as.LeftValue, primary) {
			initialized = as.ArrayMember == nil && as.HasOriginPC && (plainTryValue(as.JavaValue) == values.JavaNull || values.IsNullLiteral(plainTryValue(as.JavaValue)))
			for _, r := range typed.ProtectedRanges {
				if as.OriginPC >= r[0] {
					initialized = false
				}
			}
			break
		}
		if resourceWritesValue(as.JavaValue, primary) {
			return nil, false // a different LHS can hide a folded primary write
		}
	}
	if !initialized || resourceWritesLocal(tr.TryBody, primary) {
		return nil, false
	}
	handler := finallyWithoutEnd(tr.CatchBodies[1])
	if len(handler) != 2 {
		return nil, false
	}
	rethrow, ok := handler[1].(*statements.CustomStatement)
	if !ok || rethrow == nil || !rethrow.HasOriginPC || finallyContains(outer.ProtectedRanges, rethrow.OriginPC) || !sameTryLocal(rethrow.ThrownValue, tr.Exception[1]) {
		return nil, false
	}
	cleanup := handler[0]
	resource, normalClose, exceptionalClose, nullable, ok := resourcePrimaryCleanup(cleanup, primary)
	if !ok || !sameUnprotectedTryCall(rows, normalClose, exceptionalClose) {
		return nil, false
	}
	// On normal completion primary is its witnessed null entry value. Match a
	// plain close, resource-null guard, or the full copied conditional cleanup;
	// every invoke still needs the same receiver and decoded outside-body PC.
	matches := func(copy, _ statements.Statement) bool {
		if call, ok := resourceVoidCall(copy); ok {
			return !nullable && sameUnprotectedTryCall(rows, call, normalClose)
		}
		if local, call, ok := resourceNormalClose(copy); ok {
			return nullable && sameTryLocal(local, resource) && sameUnprotectedTryCall(rows, call, normalClose)
		}
		local, a, b, n, ok := resourcePrimaryCleanup(copy, primary)
		return ok && n == nullable && sameTryLocal(local, resource) && sameUnprotectedTryCall(rows, a, normalClose) && sameUnprotectedTryCall(rows, b, exceptionalClose)
	}
	proof := finallyProof{cleanup: []statements.Statement{cleanup}, rows: rows, excluded: tr.Exception, remaining: 512, cleanupMatch: matches}
	covered := func(pc int) bool {
		return finallyContains(typed.ProtectedRanges, pc) && finallyContains(outer.ProtectedRanges, pc)
	}
	tryBody, _, ok := proof.block(tr.TryBody, true, covered)
	if !ok {
		return nil, false
	}
	// javac precise rethrow requires the unchanged catch parameter. The removed
	// alias has exactly one assignment and one abrupt use, both in this handler.
	caught := tr.Exception[0]
	sameThrow := statements.NewCustomStatement(func(ctx *class_context.ClassContext) string { return "throw " + caught.String(ctx) }, func(old, new *utils.VariableId) { caught.ReplaceVar(old, new) })
	sameThrow.ThrownValue = caught
	sameThrow.HasOriginPC = true
	sameThrow.OriginPC = thrown.OriginPC
	return &FinallyRegion{TryBody: tryBody, Exceptions: tr.Exception[:1], CatchBodies: [][]statements.Statement{{capture, sameThrow}}, Cleanup: []statements.Statement{cleanup}}, true
}

func plainResourceRef(v values.JavaValue) *values.JavaRef {
	r, _ := plainTryValue(v).(*values.JavaRef)
	return r
}

func resourceWritesLocal(input []statements.Statement, ref *values.JavaRef) bool {
	queue := append([]statements.Statement{}, input...)
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 && len(seen) < 512 {
		st := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if st == nil || seen[st] {
			return true
		}
		seen[st] = true
		switch x := st.(type) {
		case *statements.AssignStatement:
			if sameTryLocal(x.LeftValue, ref) || resourceWritesValue(x.JavaValue, ref) {
				return true
			}
		case *values.JavaExpression:
			if resourceWritesValue(x, ref) {
				return true
			}
		case *statements.ExpressionStatement:
			if resourceWritesValue(x.Expression, ref) {
				return true
			}
		case *statements.CustomStatement:
			if x.ThrownValue != nil && resourceWritesValue(x.ThrownValue, ref) {
				return true
			}
		case *statements.SourceAssertionStatement:
			condition, call, _, known := x.SourceAssertionProtocol()
			if !known || resourceWritesValue(condition, ref) || resourceWritesValue(call, ref) {
				return true
			}
		case *statements.IfStatement:
			if resourceWritesValue(x.Condition, ref) {
				return true
			}
			queue = append(queue, x.IfBody...)
			queue = append(queue, x.ElseBody...)
		case *statements.DoWhileStatement:
			if resourceWritesValue(x.ConditionValue, ref) {
				return true
			}
			queue = append(queue, x.Body...)
		case *statements.TryCatchStatement:
			queue = append(queue, x.TryBody...)
			for _, b := range x.CatchBodies {
				queue = append(queue, b...)
			}
		}
	}
	return len(queue) != 0
}

// Prove there is no folded local definition of the primary flag. Reads are
// harmless, but assignment expressions and increments remain real writes.
func resourceWritesValue(root values.JavaValue, ref *values.JavaRef) bool {
	queue := []values.JavaValue{root}
	seen := map[values.JavaValue]bool{}
	for len(queue) > 0 && len(seen) < 512 {
		v := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if v == nil {
			return true
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		if v == values.JavaNull || values.IsNullLiteral(v) {
			continue
		}
		switch x := v.(type) {
		case *values.JavaRef:
			if x == nil || x.CustomValue != nil || x.StackVar != nil {
				return true
			}
		case *values.JavaLiteral, *values.JavaClassMember, *values.JavaClassValue:
		case *values.SlotValue:
			queue = append(queue, x.GetValue())
		case *values.FunctionCallExpression:
			queue = append(queue, x.Arguments...)
			if !x.IsStatic {
				queue = append(queue, x.Object)
			}
		case *values.NewExpression:
			queue = append(queue, x.Length...)
			queue = append(queue, x.Initializer...)
			if x.ConstructorCall != nil {
				queue = append(queue, x.ConstructorCall.Arguments...)
			}
		case *values.TernaryExpression:
			if x == nil {
				return true
			}
			queue = append(queue, x.Condition, x.TrueValue, x.FalseValue)
		case *values.CastExpression:
			queue = append(queue, x.Value)
		case *values.JavaExpression:
			if (x.Op == values.INC || x.Op == values.DEC) && len(x.Values) > 0 && sameTryLocal(x.Values[0], ref) {
				return true
			}
			queue = append(queue, x.Values...)
		case *values.AssignmentExpression:
			if sameTryLocal(x.Target, ref) {
				return true
			}
			queue = append(queue, x.Value)
		case *values.RefMember:
			queue = append(queue, x.Object)
		case *values.JavaArrayMember:
			queue = append(queue, x.Object, x.Index)
		case *values.ArrayLengthExpression:
			queue = append(queue, x.Array)
		case *values.CustomValue:
			if !x.CapturesKnown {
				return true
			}
			queue = append(queue, x.Captures...)
		default:
			return true
		}
	}
	return len(queue) != 0
}

// Keep the two cleanup alternatives and their operands explicit. The optional
// resource guard is semantically significant: a nullable resource cannot be
// substituted by an unconditional close, even if method names match.
func resourcePrimaryCleanup(st statements.Statement, primary *values.JavaRef) (*values.JavaRef, *values.FunctionCallExpression, *values.FunctionCallExpression, bool, bool) {
	branch, ok := st.(*statements.IfStatement)
	if !ok || branch == nil {
		return nil, nil, nil, false, false
	}
	nullable := false
	var guardedResource *values.JavaRef
	if local, body, guarded := resourceCloseGuard(branch); guarded && !sameTryLocal(local, primary) {
		if len(body) != 1 {
			return nil, nil, nil, false, false
		}
		inner, ok := body[0].(*statements.IfStatement)
		if !ok || inner == nil {
			return nil, nil, nil, false, false
		}
		branch = inner
		nullable = true
		guardedResource = local
	}
	conditional := *branch
	conditional.ElseBody = nil
	local, body, ok := resourceCloseGuard(&conditional)
	if !ok || !sameTryLocal(local, primary) || len(body) != 1 {
		return nil, nil, nil, false, false
	}
	inner, ok := body[0].(*statements.TryCatchStatement)
	if !ok {
		return nil, nil, nil, false, false
	}
	exceptional, ok := resourceSuppressedClose(inner, primary)
	if !ok {
		return nil, nil, nil, false, false
	}
	elseBody := finallyWithoutEnd(branch.ElseBody)
	if len(elseBody) != 1 {
		return nil, nil, nil, false, false
	}
	normal, ok := resourceVoidCall(elseBody[0])
	if !ok {
		return nil, nil, nil, false, false
	}
	resource := plainResourceRef(normal.Object)
	if resource == nil || !sameTryLocal(exceptional.Object, resource) || nullable && !sameTryLocal(resource, guardedResource) {
		return nil, nil, nil, false, false
	}
	return resource, normal, exceptional, nullable, true
}
