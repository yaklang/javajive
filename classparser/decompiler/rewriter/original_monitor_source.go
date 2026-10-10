package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A monitor catch-all releases the exact acquisition and rethrows its original
// exception. It does not catch acquisition failure or its terminal ATHROW.
func originalMonitorHandlerClosed(tr *statements.TryCatchStatement, owner int) bool {
	if tr == nil || len(tr.Handlers) != 1 || len(tr.Exception) != 1 || len(tr.CatchBodies) != 1 || tr.Exception[0] == nil {
		return false
	}
	h := tr.Handlers[0]
	if !h.CatchAll || h.EntryPC < 0 || len(h.ProtectedRanges) == 0 || len(h.ProtectedRanges) > 32 || finallyContains(h.ProtectedRanges, owner) {
		return false
	}
	for _, row := range h.ProtectedRanges {
		if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 {
			return false
		}
	}
	body := finallyWithoutEnd(tr.CatchBodies[0])
	if len(body) != 2 {
		return false
	}
	exit, ok := body[0].(*statements.MiddleStatement)
	if !ok {
		return false
	}
	pc, paired, known := exit.OriginalMonitor()
	if !known || exit.Flag != "monitor_exit" || paired != owner || pc < h.EntryPC || !finallyContains(h.ProtectedRanges, pc) {
		return false
	}
	throw, ok := body[1].(*statements.CustomStatement)
	return ok && throw != nil && throw.HasOriginPC && throw.OriginPC >= 0 && !finallyContains(h.ProtectedRanges, throw.OriginPC) && sameTryLocal(throw.ThrownValue, tr.Exception[0])
}

// A direct normal release separates the synchronized body from continuation.
// Other releases must immediately precede an inert abrupt completion, whose
// original evaluation was already performed while the monitor was held. Never
// move calls/casts/field reads across a release, or infer a loop target here.
func originalMonitorSourceBody(body []statements.Statement, owner int) ([]statements.Statement, []statements.Statement, bool) {
	end := len(body)
	var continuation []statements.Statement
	for i, st := range body {
		if x, ok := st.(*statements.MiddleStatement); ok {
			_, paired, known := x.OriginalMonitor()
			if known && x.Flag == "monitor_exit" && paired == owner {
				// An already-materialized immediate return has no evaluation
				// after release. Keep it in the lexical monitor scope so its
				// dedicated snapshot needs no synthetic default initialization.
				if i == len(body)-2 {
					if terminal, ok := body[i+1].(*statements.ReturnStatement); ok && terminal != nil && terminal.HasOriginPC && terminal.OriginPC >= 0 && (terminal.JavaValue == nil || originalMonitorInertOperand(terminal.JavaValue)) {
						continue
					}
				}
				end = i
				continuation = body[i+1:]
				break
			}
		}
	}
	remaining := 1024
	var strip func([]statements.Statement, int) ([]statements.Statement, bool)
	strip = func(input []statements.Statement, depth int) ([]statements.Statement, bool) {
		if depth > 32 {
			return nil, false
		}
		out := make([]statements.Statement, 0, len(input))
		for i, st := range input {
			remaining--
			if remaining < 0 || st == nil {
				return nil, false
			}
			switch x := st.(type) {
			case *statements.MiddleStatement:
				if x == nil {
					return nil, false
				}
				if x.Flag == "monitor_exit" {
					_, paired, known := x.OriginalMonitor()
					if !known || paired != owner || i != len(input)-2 {
						return nil, false
					}
					switch terminal := input[i+1].(type) {
					case *statements.ReturnStatement:
						if terminal == nil || !terminal.HasOriginPC || terminal.OriginPC < 0 || (terminal.JavaValue != nil && !originalMonitorInertOperand(terminal.JavaValue)) {
							return nil, false
						}
					case *statements.CustomStatement:
						if terminal == nil || !terminal.HasOriginPC || terminal.OriginPC < 0 || terminal.ThrownValue == nil || !originalMonitorInertOperand(terminal.ThrownValue) {
							return nil, false
						}
					default:
						return nil, false
					}
					continue
				}
				if x.Data != nil || (x.Flag != "start" && x.Flag != "end") {
					return nil, false
				}
			case *statements.IfStatement:
				if x == nil {
					return nil, false
				}
				clone := *x
				var ok bool
				clone.IfBody, ok = strip(x.IfBody, depth+1)
				if !ok {
					return nil, false
				}
				clone.ElseBody, ok = strip(x.ElseBody, depth+1)
				if !ok {
					return nil, false
				}
				st = &clone
			case *statements.TryCatchStatement:
				if x == nil {
					return nil, false
				}
				clone := *x
				var ok bool
				clone.TryBody, ok = strip(x.TryBody, depth+1)
				if !ok {
					return nil, false
				}
				clone.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
				for i, b := range x.CatchBodies {
					clone.CatchBodies[i], ok = strip(b, depth+1)
					if !ok {
						return nil, false
					}
				}
				st = &clone
			case *statements.SynchronizedStatement:
				if x == nil {
					return nil, false
				}
				if _, known := x.OriginalMonitorEnterPC(); !known {
					return nil, false
				}
			case *statements.AssignStatement, *statements.ExpressionStatement, *statements.ReturnStatement:
			case *statements.CustomStatement:
				if x == nil || x.ThrownValue == nil || !x.HasOriginPC {
					return nil, false
				}
			default:
				return nil, false
			}
			out = append(out, st)
		}
		return out, true
	}
	rewritten, ok := strip(body[:end], 0)
	return rewritten, continuation, ok
}

func originalMonitorInertOperand(v values.JavaValue) bool {
	for depth := 0; depth < 128; depth++ {
		if slot, ok := v.(*values.SlotValue); ok {
			if slot == nil {
				return false
			}
			v = slot.GetValue()
			continue
		}
		if v == values.JavaNull {
			return true
		}
		switch x := v.(type) {
		case *values.JavaRef:
			return x != nil && x.Id != nil && x.CustomValue == nil && x.StackVar == nil
		case *values.JavaLiteral:
			return x != nil
		}
		return false
	}
	return false
}
