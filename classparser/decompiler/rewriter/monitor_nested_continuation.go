package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A normal monitor release can be nested inside an original try/handler tree.
// Its released suffix is not part of that tree's protected domain. Retaining
// the suffix in synchronized changes locking; retaining it in the inner try
// also makes cleanup failures execute that cleanup a second time. Split a
// single proved normal frontier without introducing a dispatch variable, so
// the existing finally proof still sees its exact terminal cleanup packet.
func originalMonitorSingleContinuation(tr *statements.TryCatchStatement, owner int) (prefix, held, tail []statements.Statement, known bool) {
	if !originalMonitorHandlerClosed(tr, owner) {
		return nil, nil, nil, false
	}
	p := monitorSingleContinuationProof{owner: owner, rows: tr.Handlers[0].ProtectedRanges, remaining: 2048}
	body, open, valid := p.partition(tr.TryBody, 0)
	if !valid || open || p.releases != 1 {
		return nil, nil, nil, false
	}
	prefix, body, known = monitorPrimitiveLiveOut(body, p.tail, p.rows, owner)
	return prefix, body, p.tail, known
}

type monitorSingleContinuationProof struct {
	owner, remaining, releases int
	rows                       [][2]int
	tail                       []statements.Statement
}

func (p *monitorSingleContinuationProof) partition(input []statements.Statement, depth int) ([]statements.Statement, bool, bool) {
	if depth > 32 {
		return nil, false, false
	}
	input = finallyWithoutEnd(input)
	covered := func(pc int) bool { return finallyContains(p.rows, pc) }
	var out []statements.Statement
	for i, st := range input {
		p.remaining--
		if p.remaining < 0 || st == nil {
			return nil, false, false
		}
		switch x := st.(type) {
		case *statements.MiddleStatement:
			if x == nil {
				return nil, false, false
			}
			pc, paired, proved := x.OriginalMonitor()
			if x.Flag == "monitor_exit" {
				if !proved || paired != p.owner || !covered(pc) || p.releases != 0 {
					return nil, false, false
				}
				p.releases++
				p.tail = append([]statements.Statement(nil), input[i+1:]...)
				return out, false, true
			}
			if x.Data != nil || x.Flag != "start" && x.Flag != "end" {
				return nil, false, false
			}
		case *statements.IfStatement:
			if x == nil || !finallyCoveredValue(x.Condition, covered) {
				return nil, false, false
			}
			clone := *x
			a, aOpen, valid := p.partition(x.IfBody, depth+1)
			if !valid {
				return nil, false, false
			}
			b, bOpen, valid := p.partition(x.ElseBody, depth+1)
			if !valid {
				return nil, false, false
			}
			if aOpen != bOpen {
				rest, open, valid := p.partition(input[i+1:], depth+1)
				if !valid {
					return nil, false, false
				}
				if aOpen {
					a = append(a, rest...)
				} else {
					b = append(b, rest...)
				}
				clone.IfBody, clone.ElseBody = a, b
				return append(out, &clone), open, true
			}
			clone.IfBody, clone.ElseBody = a, b
			out = append(out, &clone)
			if !aOpen {
				return out, false, i == len(input)-1
			}
		case *statements.TryCatchStatement:
			if x == nil || len(x.Exception) == 0 || len(x.Exception) > 32 || len(x.Exception) != len(x.Handlers) || len(x.CatchBodies) != len(x.Handlers) {
				return nil, false, false
			}
			for j, h := range x.Handlers {
				if x.Exception[j] == nil || x.Exception[j].Id == nil || !covered(h.EntryPC) || !p.rangesHeld(h.ProtectedRanges) {
					return nil, false, false
				}
			}
			before := p.releases
			clone := *x
			var open, valid bool
			clone.TryBody, open, valid = p.partition(x.TryBody, depth+1)
			if !valid {
				return nil, false, false
			}
			clone.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
			for j, body := range x.CatchBodies {
				var handlerOpen bool
				clone.CatchBodies[j], handlerOpen, valid = p.partition(body, depth+1)
				if !valid {
					return nil, false, false
				}
				open = open || handlerOpen
			}
			// An unresolved arm would need to move its shared continuation
			// through a handler boundary. This single-frontier proof grants
			// no such movement. Abrupt arms need no fabricated tag value.
			if open && p.releases != before {
				return nil, false, false
			}
			parts, valid := p.handlerContinuation(&clone)
			if !valid {
				return nil, false, false
			}
			out = append(out, parts...)
			if !open {
				return out, false, i == len(input)-1
			}
		case *statements.CustomStatement:
			operand, proved := x.SourceThrowOperand()
			if !proved || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(operand, covered) || i != len(input)-1 {
				return nil, false, false
			}
			return append(out, st), false, true
		default:
			proof := finallyProof{remaining: p.remaining}
			_, _, valid := proof.block([]statements.Statement{st}, false, covered)
			p.remaining = proof.remaining
			if !valid {
				return nil, false, false
			}
			out = append(out, st)
		}
	}
	return out, true, true
}

// Every nested protected interval belongs to the still-held monitor domain.
// Thus moving a released operation out of that source try cannot remove an
// original catch edge. Unsorted/overlapping monitor rows are handled as a
// bounded interval union, rather than by scanning all bytecode offsets.
func (p *monitorSingleContinuationProof) rangesHeld(rows [][2]int) bool {
	if len(rows) == 0 || len(rows) > 32 {
		return false
	}
	for _, row := range rows {
		if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 {
			return false
		}
		for cursor := row[0]; cursor < row[1]; {
			end := cursor
			for _, held := range p.rows {
				p.remaining--
				if p.remaining < 0 {
					return false
				}
				if held[0] <= cursor && held[1] > end {
					end = held[1]
				}
			}
			if end == cursor {
				return false
			}
			cursor = end
		}
	}
	return true
}
