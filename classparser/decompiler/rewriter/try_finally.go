package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// FinallyRegion is a rendering view. The original tree remains intact for
// variable rebinding, declaration placement and all existing child visitors.
type FinallyRegion struct {
	TryBody     []statements.Statement
	Exceptions  []*values.JavaRef
	CatchBodies [][]statements.Statement
	Cleanup     []statements.Statement
}

// RecoverCatchAllFinally recognizes javac's duplicated cleanup using raw
// exception-table coverage and typed invoke identities. A sibling catch-all
// does not protect preceding catch bodies; emitting it as one silently loses
// cleanup on rethrows. Conversely, normal cleanup copies are outside the
// protected intervals, so leaving them in the try catches cleanup failures
// incorrectly. Both changes must be proved together, before any tree changes.
// Unknown control flow, opaque effects and region gaps fail closed.
func RecoverCatchAllFinally(tr *statements.TryCatchStatement) (*FinallyRegion, bool) {
	if tr == nil || len(tr.Exception) == 0 || len(tr.Exception) > 32 || len(tr.Handlers) != len(tr.Exception) || len(tr.CatchBodies) != len(tr.Exception) {
		return nil, false
	}
	last := len(tr.Handlers) - 1
	h := tr.Handlers[last]
	if !h.CatchAll || len(h.ProtectedRanges) == 0 || len(h.ProtectedRanges) > 32 {
		return nil, false
	}
	var rows []core.HandlerRange
	for i, handler := range tr.Handlers {
		if i != last && (handler.CatchAll || !finallyContains(h.ProtectedRanges, handler.EntryPC)) {
			return nil, false
		}
		if len(handler.ProtectedRanges) == 0 || len(handler.ProtectedRanges) > 32 {
			return nil, false
		}
		for _, row := range handler.ProtectedRanges {
			if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 {
				return nil, false
			}
			rows = append(rows, core.HandlerRange{StartPc: uint16(row[0]), EndPc: uint16(row[1])})
		}
	}
	body := finallyWithoutEnd(tr.CatchBodies[last])
	if len(body) < 2 || len(body) > 9 || tr.Exception[last] == nil {
		return nil, false
	}
	throw, ok := body[len(body)-1].(*statements.CustomStatement)
	if !ok || throw == nil || !throw.HasOriginPC || finallyContains(h.ProtectedRanges, throw.OriginPC) || !sameTryLocal(throw.ThrownValue, tr.Exception[last]) {
		return nil, false
	}
	cleanup := body[:len(body)-1]
	excluded := append([]*values.JavaRef{}, tr.Exception...)
	queue := append([]statements.Statement{}, tr.TryBody...)
	for _, b := range tr.CatchBodies[:last] {
		queue = append(queue, b...)
	}
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 && len(seen) < 256 {
		st := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if st == nil || seen[st] {
			return nil, false
		}
		seen[st] = true
		switch x := st.(type) {
		case *statements.AssignStatement:
			if x == nil {
				return nil, false
			}
			if ref, ok := plainTryValue(x.LeftValue).(*values.JavaRef); ok {
				excluded = append(excluded, ref)
			}
		case *statements.IfStatement:
			if x == nil {
				return nil, false
			}
			queue = append(queue, x.IfBody...)
			queue = append(queue, x.ElseBody...)
		}
	}
	if len(queue) != 0 {
		return nil, false
	}
	for _, st := range cleanup {
		call, ok := st.(*statements.ExpressionStatement)
		if !ok || call == nil || !sameUnprotectedTryCall(rows, call.Expression, call.Expression, excluded...) {
			return nil, false
		}
	}
	p := finallyProof{cleanup: cleanup, rows: rows, excluded: excluded, remaining: 256}
	// The try arm is protected by every typed catch, whereas catch bodies are
	// protected only by the raw catch-all. Preserve that distinction for folded
	// calls and casts, not just the PC of their enclosing statement.
	tryCovered := func(pc int) bool {
		for _, handler := range tr.Handlers {
			if !finallyContains(handler.ProtectedRanges, pc) {
				return false
			}
		}
		return true
	}
	tryBody, _, ok := p.block(tr.TryBody, true, tryCovered)
	if !ok {
		return nil, false
	}
	result := &FinallyRegion{TryBody: tryBody, Exceptions: tr.Exception[:last], Cleanup: cleanup}
	for _, b := range tr.CatchBodies[:last] {
		rewritten, _, ok := p.block(b, true, func(pc int) bool { return finallyContains(h.ProtectedRanges, pc) })
		if !ok {
			return nil, false
		}
		result.CatchBodies = append(result.CatchBodies, rewritten)
	}
	return result, true
}

func finallyContains(rows [][2]int, pc int) bool {
	for _, row := range rows {
		if pc >= row[0] && pc < row[1] {
			return true
		}
	}
	return false
}

func finallyWithoutEnd(body []statements.Statement) []statements.Statement {
	for len(body) > 0 {
		end, ok := body[len(body)-1].(*statements.MiddleStatement)
		if !ok || end == nil || end.Flag != "end" || end.Data != nil {
			break
		}
		body = body[:len(body)-1]
	}
	return body
}

type finallyProof struct {
	cleanup   []statements.Statement
	rows      []core.HandlerRange
	excluded  []*values.JavaRef
	remaining int
}

func (p *finallyProof) block(input []statements.Statement, mustExit bool, covered func(int) bool) ([]statements.Statement, bool, bool) {
	body := finallyWithoutEnd(input)
	end := len(body)
	var ret *statements.ReturnStatement
	if end > 0 {
		ret, _ = body[end-1].(*statements.ReturnStatement)
		if ret != nil {
			if !ret.HasOriginPC || covered(ret.OriginPC) || (ret.JavaValue != nil && !finallyPureLocal(ret.JavaValue)) {
				return nil, false, false
			}
			end--
		}
	}
	if end >= len(p.cleanup) {
		matched := true
		for i, st := range p.cleanup {
			copy, ok := body[end-len(p.cleanup)+i].(*statements.ExpressionStatement)
			original := st.(*statements.ExpressionStatement)
			if !ok || copy == nil || !sameUnprotectedTryCall(p.rows, copy.Expression, original.Expression, p.excluded...) {
				matched = false
				break
			}
		}
		if matched {
			if !mustExit {
				return nil, false, false // cleanup cannot precede a continuation
			}
			prefix, exits, ok := p.block(body[:end-len(p.cleanup)], false, covered)
			if !ok || exits {
				return nil, false, false
			}
			if ret != nil {
				prefix = append(prefix, ret)
			}
			return prefix, ret != nil, true
		}
	}
	if ret != nil {
		return nil, false, false // normal exit without its proven cleanup copy
	}
	var out []statements.Statement
	exits := false
	for i, st := range body {
		p.remaining--
		if p.remaining < 0 || st == nil || exits {
			return nil, false, false
		}
		switch x := st.(type) {
		case *statements.ExpressionStatement:
			if x == nil || !finallyCoveredValue(x.Expression, covered) {
				return nil, false, false
			}
		case *statements.AssignStatement:
			if x == nil || x.ArrayMember != nil || !finallyPureLocal(x.LeftValue) || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(x.JavaValue, covered) {
				return nil, false, false
			}
		case *statements.CustomStatement:
			if x == nil || x.ThrownValue == nil || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(x.ThrownValue, covered) {
				return nil, false, false
			}
			exits = true
		case *statements.IfStatement:
			if x == nil || !finallyCoveredValue(x.Condition, covered) {
				return nil, false, false
			}
			clone := *x
			var a, b, ok bool
			needExit := mustExit && i == len(body)-1
			clone.IfBody, a, ok = p.block(x.IfBody, needExit, covered)
			if !ok {
				return nil, false, false
			}
			clone.ElseBody, b, ok = p.block(x.ElseBody, needExit, covered)
			if !ok {
				return nil, false, false
			}
			exits = a && b
			st = &clone
		default:
			return nil, false, false
		}
		out = append(out, st)
	}
	return out, exits, !mustExit || exits
}

func finallyPureLocal(v values.JavaValue) bool {
	switch v := plainTryValue(v).(type) {
	case *values.JavaRef:
		return v != nil && v.Id != nil
	case *values.JavaLiteral:
		return v != nil
	}
	return false
}

func finallyCoveredValue(root values.JavaValue, covered func(int) bool) bool {
	active := map[values.JavaValue]bool{}
	remaining := 512
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) bool {
		remaining--
		if v == nil || remaining < 0 || active[v] {
			return false
		}
		active[v] = true
		defer delete(active, v)
		var children []values.JavaValue
		switch x := v.(type) {
		case *values.JavaRef, *values.JavaLiteral:
			return finallyPureLocal(v)
		case *values.SlotValue:
			if x == nil {
				return false
			}
			children = []values.JavaValue{x.GetValue()}
		case *values.FunctionCallExpression:
			if x == nil || !x.HasOriginPC || !covered(x.OriginPC) || x.Kind >= values.InvokeDynamic || x.Descriptor == "" || x.ClassName == "" || x.FunctionName == "" || (x.Kind == values.InvokeStatic) != x.IsStatic {
				return false
			}
			children = append(children, x.Arguments...)
			if x.IsStatic {
				// Reuse the exact static-qualifier proof without requiring a
				// void return or an unprotected PC.
				if x.Object != nil && !finallyStaticQualifier(x) {
					return false
				}
			} else {
				children = append(children, x.Object)
			}
		case *values.NewExpression:
			if x == nil {
				return false
			}
			ctor := x.ConstructorCall
			if !x.HasOriginPC || !covered(x.OriginPC) || x.IsArray() || ctor == nil || !ctor.HasOriginPC || !covered(ctor.OriginPC) || ctor.Kind != values.InvokeSpecial || ctor.FunctionName != "<init>" {
				return false
			}
			children = ctor.Arguments // ctor.Object points back to this allocation
		case *values.CastExpression:
			if x == nil || x.TargetType == nil || !covered(x.OriginPC) {
				return false
			}
			children = []values.JavaValue{x.Value}
		case *values.JavaExpression:
			if x == nil {
				return false
			}
			if (x.Op == values.Not && len(x.Values) != 1) || (x.Op != values.Not && len(x.Values) != 2) {
				return false
			}
			switch x.Op {
			case values.Not, values.EQ, values.NEQ, values.LT, values.LTE, values.GT, values.GTE, values.LOGICAL_AND, values.LOGICAL_OR:
				children = x.Values
			default:
				return false
			}
		case *values.CustomValue:
			if x == nil || x.Flag != "instanceof" || !x.CapturesKnown || len(x.Captures) != 1 || !x.HasOriginPC || !covered(x.OriginPC) {
				return false
			}
			children = x.Captures
		default:
			return false
		}
		for _, child := range children {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	return visit(root)
}
