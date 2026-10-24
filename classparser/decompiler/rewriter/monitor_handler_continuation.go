package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// Structuring can place a normal continuation inside a try whose raw JVM
// handlers do not protect it. Finally recognition is not the authority here:
// differing SSA exit flags can prevent a cleanup-copy match. Preserve the
// catch's original cleanup and split only a complete outside-handler suffix.
// This retains exception identity and prevents a normal cleanup failure from
// being caught by the handler that already executes that cleanup.
func (p *monitorSingleContinuationProof) handlerContinuation(tr *statements.TryCatchStatement) ([]statements.Statement, bool) {
	inside := func(pc int) bool {
		for _, h := range tr.Handlers {
			if !finallyContains(h.ProtectedRanges, pc) {
				return false
			}
		}
		return true
	}
	var rows [][2]int
	for _, h := range tr.Handlers {
		rows = append(rows, h.ProtectedRanges...)
	}
	outside := func(pc int) bool { return !finallyContains(rows, pc) }
	domain := func(st statements.Statement, covered func(int) bool) bool {
		// The cleanup matcher deliberately refuses a return without its
		// cleanup packet. A domain query has no such matching obligation:
		// retain only the original terminal return and its inert snapshot.
		if ret, ok := st.(*statements.ReturnStatement); ok {
			p.remaining--
			return p.remaining >= 0 && ret != nil && ret.HasOriginPC && ret.OriginPC >= 0 && covered(ret.OriginPC) && (ret.JavaValue == nil || originalMonitorInertOperand(ret.JavaValue) && finallyCoveredValue(ret.JavaValue, covered))
		}
		proof := finallyProof{remaining: p.remaining}
		_, _, known := proof.block([]statements.Statement{st}, false, covered)
		p.remaining = proof.remaining
		return known
	}
	cut := len(tr.TryBody)
	for i, st := range tr.TryBody {
		if cut == len(tr.TryBody) && domain(st, inside) {
			continue
		}
		if !domain(st, outside) {
			return nil, false // Mixed or reentering domains need another proof.
		}
		if cut == len(tr.TryBody) {
			cut = i
		}
	}
	if cut == len(tr.TryBody) {
		return []statements.Statement{tr}, true
	}
	if cut == 0 || len(rows) > 128 {
		return nil, false
	}
	clone := *tr
	clone.TryBody = tr.TryBody[:cut]
	prefix, body, known := monitorPrimitiveLiveOut([]statements.Statement{&clone}, tr.TryBody[cut:], rows, p.owner)
	if !known {
		return nil, false
	}
	out := append(prefix, body...)
	return append(out, tr.TryBody[cut:]...), true
}

// A containing monitor may already have a direct release, so its source-body
// proof never needs the single-frontier partition. Its nested tries can still
// contain outside-handler continuations. Normalize those independently after
// composing child monitors; a failed local proof leaves that subtree intact.
func normalizeOriginalMonitorHandlers(body []statements.Statement, owner int, rows [][2]int) []statements.Statement {
	p := monitorSingleContinuationProof{owner: owner, rows: rows, remaining: 4096}
	var walk func([]statements.Statement, int) []statements.Statement
	walk = func(input []statements.Statement, depth int) []statements.Statement {
		if depth > 32 || p.remaining < len(input) {
			return input
		}
		out := make([]statements.Statement, 0, len(input))
		for _, st := range input {
			p.remaining--
			switch x := st.(type) {
			case *statements.TryCatchStatement:
				if x == nil || len(x.Exception) == 0 || len(x.Exception) > 32 || len(x.Exception) != len(x.Handlers) || len(x.CatchBodies) != len(x.Handlers) {
					break
				}
				clone := *x
				clone.TryBody = walk(x.TryBody, depth+1)
				clone.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
				closed := true
				for j, h := range x.Handlers {
					clone.CatchBodies[j] = walk(x.CatchBodies[j], depth+1)
					closed = closed && x.Exception[j] != nil && finallyContains(rows, h.EntryPC) && p.rangesHeld(h.ProtectedRanges)
				}
				if closed {
					if parts, known := p.handlerContinuation(&clone); known {
						out = append(out, parts...)
						continue
					}
				}
				st = &clone
			case *statements.IfStatement:
				if x == nil {
					break
				}
				clone := *x
				clone.IfBody, clone.ElseBody = walk(x.IfBody, depth+1), walk(x.ElseBody, depth+1)
				st = &clone
			case *statements.SynchronizedStatement:
				if x == nil {
					break
				}
				clone := *x
				clone.Body = walk(x.Body, depth+1)
				st = &clone
			case *statements.DoWhileStatement:
				if x == nil {
					break
				}
				clone := *x
				clone.Body = walk(x.Body, depth+1)
				st = &clone
			}
			out = append(out, st)
		}
		return out
	}
	return walk(body, 0)
}
