package rewriter

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A structured branch can contain several releases followed by different
// continuations. Keeping those continuations inside synchronized changes lock
// ownership; dropping the releases does not establish a Java source boundary.
// Partition at the original CFG-certified releases and dispatch their retained
// tails after the monitor. The private tag records only which original edge
// was taken. This typed plan declares it without an initializer: every normally
// completing held path must assign it. No condition, call, cleanup or algorithm
// result is recomputed.
func originalMonitorContinuations(tr *statements.TryCatchStatement, owner int) (prefix, held, tail []statements.Statement, known bool) {
	if !originalMonitorHandlerClosed(tr, owner) {
		return nil, nil, nil, false
	}
	p := monitorContinuationProof{owner: owner, rows: tr.Handlers[0].ProtectedRanges, remaining: 1024}
	p.tag = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	p.tag.Id.SetName(fmt.Sprintf("jdec$monitor$exit$%d", owner))
	body, open, valid := p.partition(tr.TryBody, 0)
	if !valid || open || len(p.tails) < 2 || len(p.tails) > 32 || !monitorContinuationLocalsClosed(body, p.tails, func(pc int) bool { return !finallyContains(p.rows, pc) }) {
		return nil, nil, nil, false
	}
	continuation := p.tails[len(p.tails)-1]
	for i := len(p.tails) - 2; i >= 0; i-- {
		condition := values.NewBinaryExpression(p.tag, values.NewJavaLiteral(i, p.tag.Type()), values.EQ, types.NewJavaPrimer(types.JavaBoolean))
		continuation = []statements.Statement{statements.NewIfStatement(condition, p.tails[i], continuation)}
	}
	return []statements.Statement{statements.NewDeclareStatement(p.tag)}, body, continuation, true
}

type monitorContinuationProof struct {
	owner, remaining int
	rows             [][2]int
	tag              *values.JavaRef
	tails            [][]statements.Statement
}

// open means control still holds this acquisition at the end of the list.
// When just one branch remains open, the shared suffix belongs only to it;
// attach it there once. Closed arms already recorded their exact continuation.
func (p *monitorContinuationProof) partition(input []statements.Statement, depth int) ([]statements.Statement, bool, bool) {
	if depth > 32 {
		return nil, false, false
	}
	input = finallyWithoutEnd(input)
	var out []statements.Statement
	covered := func(pc int) bool { return finallyContains(p.rows, pc) }
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
				if !proved || paired != p.owner || !covered(pc) || len(p.tails) >= 32 {
					return nil, false, false
				}
				store := statements.NewAssignStatement(p.tag, values.NewJavaLiteral(len(p.tails), p.tag.Type()), false)
				store.HasOriginPC, store.OriginPC = true, pc
				p.tails = append(p.tails, append([]statements.Statement{}, input[i+1:]...))
				return append(out, store), false, true
			}
			if x.Data != nil || x.Flag != "start" && x.Flag != "end" {
				return nil, false, false
			}
			continue
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
		case *statements.CustomStatement:
			operand, proved := x.SourceThrowOperand()
			if !proved || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(operand, covered) || i != len(input)-1 {
				return nil, false, false
			}
			return append(out, st), false, true
		default:
			// This proof never removes or reorders a held operation. Reuse the
			// exact effect-domain visitor, with no cleanup to recognize or move.
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

// A tail must not acquire a use of a local declared inside synchronized. This
// partition deliberately refuses such a cross-scope value; it does not invent
// initialization values or use a simulator ref.Val history as a declaration.
func monitorContinuationLocalsClosed(held []statements.Statement, tails [][]statements.Statement, released func(int) bool) bool {
	declared := map[*utils.VariableId]bool{}
	remaining := 4096
	active := map[statements.Statement]bool{}
	var value func(values.JavaValue, map[values.JavaValue]bool) bool
	value = func(v values.JavaValue, seen map[values.JavaValue]bool) bool {
		remaining--
		if remaining < 0 || v == nil || seen[v] {
			return false
		}
		seen[v] = true
		defer delete(seen, v)
		if ref, ok := v.(*values.JavaRef); ok && declared[ref.Id] {
			return false
		}
		children, known := values.Children(v)
		if call, ok := v.(*values.FunctionCallExpression); ok {
			children = append([]values.JavaValue{}, call.Arguments...)
			if !call.IsStatic {
				children = append(children, call.Object)
			}
			known = true // the separate effect-domain proof validates this call
		}
		if allocation, ok := v.(*values.NewExpression); ok && !allocation.IsArray() && allocation.ConstructorCall != nil {
			children, known = allocation.ConstructorCall.Arguments, true
		}
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child, seen) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement, bool) bool
	walk = func(body []statements.Statement, collect bool) bool {
		for _, st := range body {
			remaining--
			if remaining < 0 || st == nil || active[st] {
				return false
			}
			active[st] = true
			var roots []values.JavaValue
			var blocks [][]statements.Statement
			switch x := st.(type) {
			case *statements.AssignStatement:
				if !collect && !finallyCoveredAssignment(x, released) {
					return false
				}
				if ref, ok := x.LeftValue.(*values.JavaRef); collect && ok && (x.IsFirst || x.IsDeclare) {
					if ref == nil || ref.Id == nil {
						return false
					}
					declared[ref.Id] = true
				}
				if x.ArrayMember != nil {
					roots = append(roots, x.ArrayMember)
				} else if x.LeftValue != nil {
					roots = append(roots, x.LeftValue)
				}
				if x.JavaValue != nil {
					roots = append(roots, x.JavaValue)
				}
			case *statements.ExpressionStatement:
				roots = []values.JavaValue{x.Expression}
			case *values.JavaExpression:
				roots = []values.JavaValue{x}
			case *statements.IfStatement:
				roots, blocks = []values.JavaValue{x.Condition}, [][]statements.Statement{x.IfBody, x.ElseBody}
			case *statements.TryCatchStatement:
				blocks = append([][]statements.Statement{x.TryBody}, x.CatchBodies...)
			case *statements.ReturnStatement:
				if !collect && (!x.HasOriginPC || !released(x.OriginPC)) {
					return false
				}
				if x.JavaValue != nil {
					roots = []values.JavaValue{x.JavaValue}
				}
			case *statements.CustomStatement:
				if x.ThrownValue != nil {
					if !collect && (!x.HasOriginPC || !released(x.OriginPC)) {
						return false
					}
					roots = []values.JavaValue{x.ThrownValue}
				} else if !x.SourceTransferOnly() {
					return false
				}
			case *statements.SwitchStatement:
				roots = []values.JavaValue{x.Value}
				for _, arm := range x.Cases {
					if arm == nil {
						return false
					}
					blocks = append(blocks, arm.Body)
				}
			case *statements.MiddleStatement:
				if x == nil || x.Data != nil || x.Flag != "start" && x.Flag != "end" {
					return false
				}
			default:
				return false
			}
			if !collect {
				for _, root := range roots {
					if !finallyCoveredValue(root, released) || !value(root, map[values.JavaValue]bool{}) {
						return false
					}
				}
			}
			for _, block := range blocks {
				if !walk(block, collect) {
					return false
				}
			}
			delete(active, st)
		}
		return true
	}
	if !walk(held, true) {
		return false
	}
	for _, tail := range tails {
		if !walk(tail, false) {
			return false
		}
	}
	return true
}
