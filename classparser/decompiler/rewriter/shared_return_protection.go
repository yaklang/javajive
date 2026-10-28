package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// Keep original ordered exception rows independently of synthetic try wrappers.
// javac can split one protected source body around each nonthrowing RETURN;
// wrapper intervals alone cannot describe the handlers of an inlined operand.
func (manager *RewriteManager) SetOriginalExceptionTable(rows []*core.ExceptionTableEntry) {
	manager.originalHandlers, manager.originalHandlersKnown = nil, false
	if len(rows) > 4096 {
		return
	}
	copy := make([]core.HandlerRange, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.EndPc <= row.StartPc {
			return
		}
		copy = append(copy, core.HandlerRange{StartPc: row.StartPc, EndPc: row.EndPc, HandlerPc: row.HandlerPc, CatchType: row.CatchType})
	}
	manager.originalHandlers, manager.originalHandlersKnown = copy, true
}

// A shared RETURN may lie just outside a protected interval even though every
// operation computing its operand lies inside. Factoring that exact normal
// destination must preserve the operand's ordered handler chain, not pretend
// its effectful calls execute at the RETURN's nonthrowing PC. No expression is
// copied, changed, or moved across the original control-flow destination.
func sharedReturnOperandProtection(manager *RewriteManager, owner, target *core.Node) bool {
	if manager == nil || !manager.originalHandlersKnown || owner == nil || target == nil || !owner.HasOriginPC || !target.HasOriginPC || owner.OriginPC < 0 || owner.OriginPC >= target.OriginPC || target.OriginPC > 65535 {
		return false
	}
	ret, ok := target.Statement.(*statements.ReturnStatement)
	if !ok || ret == nil || !ret.HasOriginPC || ret.OriginPC != target.OriginPC || ret.JavaValue == nil {
		return false
	}
	for _, next := range target.Next {
		if !IsEndNode(next) {
			return false
		}
	}
	type handler struct{ pc, kind uint16 }
	var expected []handler
	for _, row := range manager.originalHandlers {
		if owner.OriginPC >= int(row.StartPc) && owner.OriginPC < int(row.EndPc) {
			expected = append(expected, handler{row.HandlerPc, row.CatchType})
		}
	}
	if len(expected) == 0 {
		return false
	}
	remaining := 65536
	checked := map[int]bool{}
	covered := func(pc int) bool {
		if pc < owner.OriginPC || pc >= target.OriginPC {
			return false
		}
		if yes, seen := checked[pc]; seen {
			return yes
		}
		remaining -= len(manager.originalHandlers) + 1
		if remaining < 0 {
			return false
		}
		index := 0
		for _, row := range manager.originalHandlers {
			if pc >= int(row.StartPc) && pc < int(row.EndPc) {
				if index >= len(expected) || expected[index] != (handler{row.HandlerPc, row.CatchType}) {
					checked[pc] = false
					return false
				}
				index++
			}
		}
		checked[pc] = index == len(expected)
		return checked[pc]
	}
	return finallyCoveredValue(ret.JavaValue, covered)
}
