package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A materialized boolean can keep the compiler flag and failure predicate in
// separate source branches. A single-child chain with a proved common continuation is an ordered AND:
// each predicate runs only after every preceding predicate succeeds. Fold only
// that chain, without moving an effect, handler, or false-arm continuation.
// The caller still proves the original flag read and exact allocation/call/throw
// instruction identities; source shape alone never certifies an assertion.
func nativeAssertionFailureArm(guard *statements.IfStatement, failure values.JavaValue, packet *nativeAssertionPacket, work *workbudget.Budget) (*statements.CustomStatement, values.JavaValue, bool) {
	if guard == nil {
		return nil, nil, false
	}
	body := guard.IfBody
	seen := map[statements.Statement]bool{}
	for depth := 0; depth < 64; depth++ {
		if len(body) != 1 || sourceProofNil(body[0]) || sourceProofNil(failure) || !nativeProofWork(work, 1) || seen[body[0]] {
			return nil, nil, false
		}
		st := body[0]
		seen[st] = true
		if thrown, ok := st.(*statements.CustomStatement); ok {
			_, sealed := thrown.SourceThrowOperand()
			return thrown, failure, sealed
		}
		branch, ok := st.(*statements.IfStatement)
		if !ok || !nativeAssertionSameFalseContinuation(guard.ElseBody, branch.ElseBody, packet, work) {
			return nil, nil, false
		}
		condition, ok := nativeMemberEnclosingUnpack(branch.Condition, work)
		if !ok {
			return nil, nil, false
		}
		failure = values.NewBinaryExpression(failure, condition, values.LOGICAL_AND, types.NewJavaPrimer(types.JavaBoolean))
		body = branch.IfBody
	}
	return nil, nil, false
}

// Structuring may duplicate the same terminal void return into both false arms.
// Only its physical original disabled-edge destination permits coalescing it.
// Equal printed text, different return sites, values, effects and missing source
// origins cannot stand in for this control-flow certificate.
func nativeAssertionSameFalseContinuation(outer, inner []statements.Statement, packet *nativeAssertionPacket, work *workbudget.Budget) bool {
	if !nativeProofWork(work, 1) {
		return false
	}
	if len(outer) == 0 && len(inner) == 0 {
		return true
	}
	if packet == nil || !packet.voidReturnJoin || len(outer) != 1 || len(inner) != 1 {
		return false
	}
	for _, body := range [][]statements.Statement{outer, inner} {
		r, ok := body[0].(*statements.ReturnStatement)
		if !ok || r == nil || r.JavaValue != nil || !r.HasOriginPC || r.OriginPC != packet.joinPC {
			return false
		}
	}
	return true
}
