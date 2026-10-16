package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// An internal integral switch retains its selector, case order, fall-through
// and every original statement. Only its effect domain is queried. A sealed
// unlabeled break has no operands and exits the closest retained switch.
// Returns, labels, continues, nested loops and handler scopes need their own
// exit/cleanup certificates and remain outside this permission.
func (p *finallyProof) internalSwitchCovered(root *statements.SwitchStatement, covered func(int) bool) bool {
	active := map[statements.Statement]bool{}
	var block func([]statements.Statement, int) bool
	var selection func(*statements.SwitchStatement, int) bool
	selection = func(sw *statements.SwitchStatement, depth int) bool {
		if sw == nil || sw.Value == nil || sw.Value.Type() == nil || len(sw.Cases) > 64 || depth > 24 || !finallyCoveredValue(sw.Value, covered) {
			return false
		}
		typ, known := sw.Value.Type().RawType().(*types.JavaPrimer)
		if !known || typ == nil {
			return false
		}
		switch typ.Name {
		case types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger:
		default:
			return false
		}
		keys, defaults := map[int]bool{}, 0
		for _, arm := range sw.Cases {
			if arm == nil {
				return false
			}
			if arm.IsDefault {
				defaults++
				if defaults > 1 {
					return false
				}
			} else {
				if arm.IntValue < -1<<31 || arm.IntValue > 1<<31-1 || keys[arm.IntValue] {
					return false
				}
				keys[arm.IntValue] = true
			}
			if !block(arm.Body, depth+1) {
				return false
			}
		}
		return true
	}
	block = func(body []statements.Statement, depth int) bool {
		if depth > 24 {
			return false
		}
		for _, st := range finallyWithoutEnd(body) {
			p.remaining--
			if p.remaining < 0 || st == nil || active[st] {
				return false
			}
			active[st] = true
			valid := false
			switch x := st.(type) {
			case *statements.AssignStatement:
				valid = finallyCoveredAssignment(x, covered)
			case *statements.ExpressionStatement:
				valid = x != nil && finallyCoveredValue(x.Expression, covered)
			case *values.JavaExpression:
				valid = x != nil && (x.Op == values.INC || x.Op == values.DEC) && finallyCoveredValue(x, covered)
			case *statements.SourceAssertionStatement:
				condition, call, pc, known := x.SourceAssertionProtocol()
				valid = known && covered(pc) && finallyCoveredValue(condition, covered) && finallyCoveredValue(call, covered)
			case *statements.IfStatement:
				valid = x != nil && finallyCoveredValue(x.Condition, covered) && block(x.IfBody, depth+1) && block(x.ElseBody, depth+1)
			case *statements.SwitchStatement:
				valid = selection(x, depth+1)
			case *statements.CustomStatement:
				if kind, label, known := x.SourceTransferProtocol(); known {
					valid = kind == "break" && label == ""
				} else if operand, known := x.SourceThrowOperand(); known {
					valid = x.HasOriginPC && covered(x.OriginPC) && finallyCoveredValue(operand, covered)
				}
			}
			delete(active, st)
			if !valid {
				return false
			}
		}
		return true
	}
	return selection(root, 0)
}
