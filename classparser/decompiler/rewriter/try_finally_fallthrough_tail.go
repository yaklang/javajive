package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A normal cleanup copy can be followed by another resource phase instead of
// a method return. Leaving that continuation in the first protected body makes
// a normal close failure enter its own exceptional cleanup and close twice.
// Find the boundary only when the complete prefix independently proves its
// original finally protocol, and EVERY effect and nested handler in the suffix
// lies outside all of this region's original handler intervals. The normal
// cleanup stays in the proof input; the existing finally view removes its copy
// while preserving primary/suppressed exception identity on every abrupt exit.
// No operation is replayed and no initializer or pending return is fabricated.
func factorFinallyFallthroughTail(tr *statements.TryCatchStatement) []statements.Statement {
	if tr == nil || len(tr.Handlers) == 0 || len(tr.Handlers) > 32 || len(tr.Handlers) != len(tr.Exception) || len(tr.Handlers) != len(tr.CatchBodies) {
		return nil
	}
	body := finallyWithoutEnd(tr.TryBody)
	if len(body) == 0 || len(body) > 128 {
		return nil
	}
	if _, complete := recoverFinallyDirectSource(tr); complete {
		return nil
	}
	var rows [][2]int
	for _, handler := range tr.Handlers {
		if len(handler.ProtectedRanges) == 0 || len(handler.ProtectedRanges) > 32 {
			return nil
		}
		rows = append(rows, handler.ProtectedRanges...)
	}
	ranges, known := canonicalHandlerRanges(rows)
	if !known {
		return nil
	}
	outside := func(pc int) bool { return pc >= 0 && !finallyContains(ranges, pc) }
	for split := 2; split < len(body); split++ {
		proof := outsideHandlerDomainProof(ranges)
		if !proof.block(body[split:], outside, 0) {
			continue
		}
		prefix := *tr
		prefix.TryBody = slices.Clone(body[:split])
		view, complete := recoverFinallyDirectSource(&prefix)
		if !complete || view == nil || len(view.Cleanup) == 0 || view.Continuation != nil {
			continue
		}
		// The view above is a proof, not a replacement for the typed tree.
		// Existing scope/declaration and child visitors still inspect that tree.
		tr.TryBody = prefix.TryBody
		return slices.Clone(body[split:])
	}
	return factorFinallyConditionalContinuation(tr, body, ranges)
}

// Factoring a tail creates new CFG nodes for the same already-proved source
// statements. An enclosing region can retain a boundary pointer to the old
// consumed node. Reopen that semantic reference only for one exact statement
// identity and the existing full outside-domain proof; matching PCs, printed
// source, variable IDs or an equivalent-looking copy cannot grant ownership.
func retargetFactoredStatementBoundary(regions, consumed []*core.Node, replacement *core.Node) {
	if replacement == nil || replacement.Statement == nil {
		return
	}
	var original *core.Node
	for _, node := range consumed {
		if node != nil && node != replacement && node.Statement == replacement.Statement {
			if original != nil {
				return
			}
			original = node
		}
	}
	if original == nil {
		return
	}
	replacement.HasOriginPC, replacement.OriginPC = original.HasOriginPC, original.OriginPC
	retargetProtectedSourceBoundary(regions, original, replacement)
}
