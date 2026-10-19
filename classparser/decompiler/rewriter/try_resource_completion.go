package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// Enclosing finally views remove their own cleanup copies before rendering a
// nested resource region. Only at that point may its close be the exact normal
// suffix. Reattempt the existing complete resource proof on a private clone;
// neither the original handler tree nor any earlier analysis is changed.
func RecoverCatchAllFinallyForSource(tr *statements.TryCatchStatement) (*FinallyRegion, bool) {
	if region, known := RecoverCatchAllFinally(tr); known {
		return region, true
	}
	if tr == nil {
		return nil, false
	}
	clone := *tr
	declaration := prepareTypedResourceFinally(&clone)
	if declaration == nil {
		return nil, false
	}
	region, known := recoverTypedResourceFinally(&clone)
	if !known {
		return nil, false
	}
	region.SourcePrologue = []statements.Statement{declaration}
	return region, true
}

// A single typed Throwable handler can encode resource cleanup without a raw catch-all.
// Preserve pending returns rather than factoring them across nested cleanup.
// This private state is a source-level completion carrier, never a new throw.
func prepareTypedResourceFinally(tr *statements.TryCatchStatement) *statements.AssignStatement {
	if tr == nil || len(tr.Exception) != 1 || len(tr.Handlers) != 1 || len(tr.CatchBodies) != 1 || len(finallyWithoutEnd(tr.CatchBodies[0])) != 2 {
		return nil
	}
	primary := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
	declaration := &statements.AssignStatement{LeftValue: primary, JavaValue: values.NewJavaLiteral("null", primary.Type().Copy()), IsFirst: true}
	// This is an initialized source definition, not a bare declaration.
	capture := &statements.AssignStatement{LeftValue: primary, JavaValue: tr.Exception[0]}
	clone := *tr
	clone.EntryInitializers = append([]*statements.AssignStatement{declaration}, tr.EntryInitializers...)
	clone.CatchBodies = [][]statements.Statement{append([]statements.Statement{capture}, tr.CatchBodies[0]...)}

	if _, ok := recoverTypedResourceFinally(&clone); !ok {
		return nil
	}
	tr.EntryInitializers = clone.EntryInitializers
	tr.CatchBodies = clone.CatchBodies
	return declaration
}

func recoverTypedResourceFinally(tr *statements.TryCatchStatement) (*FinallyRegion, bool) {
	if tr == nil || len(tr.Exception) != 1 || len(tr.Handlers) != 1 || len(tr.CatchBodies) != 1 || tr.Exception[0] == nil || tr.Handlers[0].CatchAll {
		return nil, false
	}
	if !resourceExactThrowable(tr.Exception[0].Type()) {
		return nil, false
	}
	h := tr.Handlers[0]
	if h.EntryPC < 0 || finallyContains(h.ProtectedRanges, h.EntryPC) || len(h.ProtectedRanges) == 0 || len(h.ProtectedRanges) > 32 {
		return nil, false
	}
	var rows []core.HandlerRange
	for _, r := range h.ProtectedRanges {
		if r[0] < 0 || r[0] >= r[1] || r[1] > 65535 {
			return nil, false
		}
		rows = append(rows, core.HandlerRange{StartPc: uint16(r[0]), EndPc: uint16(r[1])})
	}
	handler := finallyWithoutEnd(tr.CatchBodies[0])
	if len(handler) != 3 {
		return nil, false
	}
	capture, ok := handler[0].(*statements.AssignStatement)
	if !ok || capture == nil || capture.ArrayMember != nil || capture.HasOriginPC || capture.IsDeclare || capture.IsFirst || !sameTryLocal(capture.JavaValue, tr.Exception[0]) {
		return nil, false
	}
	primary := plainResourceRef(capture.LeftValue)
	if primary == nil || primary.IsParam || primary.IsThis || !finallyPureLocal(primary) || !resourceExactThrowable(primary.Type()) || sameTryLocal(primary, tr.Exception[0]) {
		return nil, false
	}
	initialized := false
	for _, a := range tr.EntryInitializers {
		if a == nil {
			return nil, false
		}
		if sameTryLocal(a.LeftValue, primary) {
			initialized = !a.IsDeclare && !a.HasOriginPC && a.ArrayMember == nil && (plainTryValue(a.JavaValue) == values.JavaNull || values.IsNullLiteral(plainTryValue(a.JavaValue)))
			break
		}
		if resourceWritesValue(a.JavaValue, primary) {
			return nil, false
		}
	}
	if !initialized || resourceWritesLocal(tr.TryBody, primary) {
		return nil, false
	}
	rethrow, ok := handler[2].(*statements.CustomStatement)
	if !ok || rethrow == nil || !rethrow.HasOriginPC || rethrow.OriginPC < 0 || finallyContains(h.ProtectedRanges, rethrow.OriginPC) || !sameTryLocal(rethrow.ThrownValue, tr.Exception[0]) {
		return nil, false
	}
	resource, exceptional, ok := resourceTailExceptionalClose(handler[1], tr.Exception[0])
	if !ok || exceptional.OriginPC < 0 || sameTryLocal(resource, tr.Exception[0]) || !finallyPureLocal(resource) {
		return nil, false
	}
	// A resource declared by the protected body would not be in finally scope.
	visible := resource.IsParam || resource.IsThis
	for _, a := range tr.EntryInitializers {
		if a != nil && a.HasOriginPC && sameTryLocal(a.LeftValue, resource) {
			visible = true
			for _, r := range h.ProtectedRanges {
				if a.OriginPC >= r[0] {
					visible = false
				}
			}
			break
		}
	}
	if !visible {
		return nil, false
	}
	_, _, exceptionGuarded := resourceCloseGuard(handler[1])
	var normal statements.Statement
	matches := func(copy, _ statements.Statement) bool {
		r, c, guarded, valid := resourceTailNormalClose(copy)
		if !valid || c.OriginPC < 0 || guarded != exceptionGuarded || !sameTryLocal(r, resource) || !sameUnprotectedTryCall(rows, c, exceptional) {
			return false
		}
		if normal == nil {
			normal = copy
		}
		return true
	}
	proof := finallyProof{cleanup: []statements.Statement{handler[1]}, rows: rows, excluded: tr.Exception, remaining: 512, cleanupMatch: matches}
	body, _, ok := proof.block(tr.TryBody, true, func(pc int) bool { return finallyContains(h.ProtectedRanges, pc) })
	if !ok || normal == nil {
		return nil, false
	}
	// Clone only the already-proved suppression protocol. Its catch binder and
	// original invoke PCs stay intact; only the primary's lifetime is extended.
	exceptionalBody := handler[1]
	var inner *statements.TryCatchStatement
	if exceptionGuarded {
		g := handler[1].(*statements.IfStatement)
		inner = g.IfBody[0].(*statements.TryCatchStatement)
	} else {
		inner = handler[1].(*statements.TryCatchStatement)
	}
	closeHandler := inner.Handlers[0]
	if closeHandler.EntryPC < 0 || finallyContains(closeHandler.ProtectedRanges, closeHandler.EntryPC) {
		return nil, false
	}
	caughtExpression := finallyWithoutEnd(inner.CatchBodies[0])[0].(*statements.ExpressionStatement)
	caughtSink := plainTryValue(caughtExpression.Expression).(*values.FunctionCallExpression)
	if caughtSink.OriginPC < 0 || finallyContains(closeHandler.ProtectedRanges, caughtSink.OriginPC) {
		return nil, false
	}
	innerCopy := *inner
	expression := finallyWithoutEnd(inner.CatchBodies[0])[0].(*statements.ExpressionStatement)
	sink := *plainTryValue(expression.Expression).(*values.FunctionCallExpression)
	sink.Object = primary
	expressionCopy := *expression
	expressionCopy.Expression = &sink
	innerCopy.CatchBodies = [][]statements.Statement{{&expressionCopy}}
	if exceptionGuarded {
		guard := *handler[1].(*statements.IfStatement)
		guard.IfBody = []statements.Statement{&innerCopy}
		exceptionalBody = &guard
	} else {
		exceptionalBody = &innerCopy
	}
	cleanup := statements.NewIfStatement(values.NewBinaryExpression(primary, values.JavaNull, values.NEQ, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{exceptionalBody}, []statements.Statement{normal})
	return &FinallyRegion{TryBody: body, Exceptions: tr.Exception, CatchBodies: [][]statements.Statement{{capture, rethrow}}, Cleanup: []statements.Statement{cleanup}}, true
}
func resourceExactThrowable(t types.JavaType) bool {
	if t == nil {
		return false
	}
	n, ok := types.RawClassFQN(t)
	return ok && strings.ReplaceAll(n, "/", ".") == "java.lang.Throwable"
}
