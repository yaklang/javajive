package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A cleanup may itself contain a protected cleanup. Compare the original
// region trees, rather than flattening either exception domain or requiring
// their compiler-created catch variables to have the same local identity.
// Only the terminal rethrow is alpha-bound: all real effects, receivers and
// other locals must still match. The rendering view retains the original tree.
func (p *finallyCleanupComparison) nested(rows []core.HandlerRange, a, b *statements.TryCatchStatement, excluded []*values.JavaRef, depth int) bool {
	if depth > 16 {
		return false
	}
	x, xknown := nestedFinallyCleanupParts(rows, a)
	y, yknown := nestedFinallyCleanupParts(rows, b)
	if !xknown || !yknown || len(a.TryBody) != len(b.TryBody) || len(x) != len(y) {
		return false
	}
	excluded = append(append([]*values.JavaRef(nil), excluded...), a.Exception[0], b.Exception[0])
	for i, st := range a.TryBody {
		if !p.statement(rows, st, b.TryBody[i], excluded, depth) {
			return false
		}
	}
	for i, st := range x {
		if !p.statement(rows, st, y[i], excluded, depth) {
			return false
		}
	}
	return true
}

func nestedFinallyCleanupParts(rows []core.HandlerRange, tr *statements.TryCatchStatement) ([]statements.Statement, bool) {
	if tr == nil || len(tr.Exception) != 1 || len(tr.Handlers) != 1 || len(tr.CatchBodies) != 1 || !nestedCleanupPredecessorMetadata(tr.EntryInitializers) || len(tr.TryBody) == 0 || len(tr.TryBody) > 16 || tr.Exception[0] == nil || tr.Exception[0].IsParam || tr.Exception[0].IsThis || !finallyPureLocal(tr.Exception[0]) || !resourceExactThrowable(tr.Exception[0].Type()) {
		return nil, false
	}
	h := tr.Handlers[0]
	if !h.CatchAll || h.EntryPC < 0 || h.EntryPC > 65535 || len(h.ProtectedRanges) == 0 || len(h.ProtectedRanges) > 32 {
		return nil, false
	}
	for _, row := range h.ProtectedRanges {
		if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 {
			return nil, false
		}
	}
	outside := func(pc int) bool {
		if pc < 0 || pc > 65535 {
			return false
		}
		for _, row := range rows {
			if int(row.StartPc) <= pc && pc < int(row.EndPc) {
				return false
			}
		}
		return true
	}
	body := finallyWithoutEnd(tr.CatchBodies[0])
	if !outside(h.EntryPC) || len(body) == 0 || len(body) > 9 {
		return nil, false
	}
	cleanup, known := finallyHandlerCleanup(body, tr.Exception[0], h.ProtectedRanges)
	if !known || len(cleanup) == 0 {
		return nil, false
	}
	// Every original evaluation, including discarded duplicate cleanup and
	// the synthetic rethrow, must remain outside the enclosing region. Inside
	// this region the try and handler have different protected domains.
	proof := finallyProof{remaining: 256}
	if _, _, known := proof.block(tr.TryBody, false, func(pc int) bool {
		return outside(pc) && finallyContains(h.ProtectedRanges, pc)
	}); !known {
		return nil, false
	}
	if _, _, known := proof.block(body, false, func(pc int) bool {
		return outside(pc) && !finallyContains(h.ProtectedRanges, pc)
	}); !known {
		return nil, false
	}
	return cleanup, true
}

// EntryInitializers records original private predecessor stores; it is not a
// source prologue executed on entry to this catch-all. Keep that provenance
// intact without moving, matching or replaying it as part of cleanup. It grants
// no initial-value fact here. Source-only resource carriers without decoded PCs
// remain outside this proof, which accepts only an original raw catch-all.
func nestedCleanupPredecessorMetadata(initializers []*statements.AssignStatement) bool {
	if len(initializers) > 32 {
		return false
	}
	for _, a := range initializers {
		if a == nil || !a.HasOriginPC || a.OriginPC < 0 || a.OriginPC > 65535 || a.ArrayMember != nil {
			return false
		}
		ref, known := a.LeftValue.(*values.JavaRef)
		if !known || !finallyPureLocal(ref) {
			return false
		}
	}
	return true
}
