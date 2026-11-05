package core

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

type loweredTryDispatch struct {
	anchor *Node
	pc     int
	rows   []HandlerRange
}

func catchDispatchRow(info *CatchNode) (HandlerRange, bool) {
	if info == nil || info.OpCode == nil || info.StartIndex >= info.EndIndex {
		return HandlerRange{}, false
	}
	return HandlerRange{info.StartIndex, info.EndIndex, info.OpCode.CurrentOffset, info.ExceptionTypeIndex}, true
}

// HasPendingTryDispatch distinguishes an original exception-dispatch anchor
// from a live dispatch. Lowering moves exception edges onto explicit try nodes;
// the predecessor can still be an ordinary condition with two normal outcomes.
// Keep IsTryCatch for earlier passes that depend on the historical anchor.
func (n *Node) HasPendingTryDispatch() bool {
	if n == nil || !n.IsTryCatch {
		return false
	}
	w := n.tryDispatchLowered
	if w == nil || w.anchor != n || !n.HasOriginPC || n.OriginPC != w.pc || len(n.CatchNodeInfo) != len(w.rows) {
		return true
	}
	for i, info := range n.CatchNodeInfo {
		if row, ok := catchDispatchRow(info); !ok || row != w.rows[i] {
			return true
		}
	}
	return false
}

// Record completion only after every attached original row is represented by
// a range-matched explicit try and handler, and no handler edge remains on the
// anchor. This certifies a lowering phase, not permission to move protected
// effects: consumers must still check live regions and exception domains.
func (n *Node) recordLoweredTryDispatch(regions []*Node, table []*ExceptionTableEntry, ordered bool) {
	if n == nil {
		return
	}
	n.tryDispatchLowered = nil
	if !ordered || !n.IsTryCatch || !n.HasOriginPC || n.OriginPC < 0 || n.OriginPC > 65535 || len(n.CatchNodeInfo) == 0 || len(n.CatchNodeInfo) > 1024 || len(regions) == 0 || len(regions) > 1024 || len(table) > 65536 {
		return
	}
	w := &loweredTryDispatch{anchor: n, pc: n.OriginPC}
	original := map[HandlerRange]bool{}
	for _, row := range table {
		if row == nil {
			return
		}
		original[HandlerRange{row.StartPc, row.EndPc, row.HandlerPc, row.CatchType}] = true
	}
	work := 65536
	expected, starts := map[HandlerRange]bool{}, map[uint16]bool{}
	for _, info := range n.CatchNodeInfo {
		row, ok := catchDispatchRow(info)
		if !ok || !original[row] {
			return
		}
		w.rows = append(w.rows, row)
		expected[row], starts[row.StartPc] = true, true
		covered := false
		for _, region := range regions {
			work--
			if work < 0 || region == nil {
				return
			}
			middle, ok := region.Statement.(*statements.MiddleStatement)
			if !ok || middle.Flag != statements.MiddleTryStart || !region.HasProtectedRange || region.ProtectedStartPC >= region.ProtectedEndPC {
				return
			}
			if region.ProtectedStartPC != int(row.StartPc) || region.ProtectedEndPC != int(row.EndPc) {
				continue
			}
			body, handler := false, false
			for _, next := range region.Next {
				work--
				if work < 0 || next == nil {
					return
				}
				if !next.IsCatchStart {
					body = true
					continue
				}
				witness := next.CatchHandler
				if witness != nil && witness.EntryPC == int(row.HandlerPc) && slices.Contains(witness.ProtectedRanges, [2]int{int(row.StartPc), int(row.EndPc)}) {
					if slices.Contains(n.Next, next) {
						return
					}
					handler = true
				}
			}
			covered = covered || body && handler
		}
		if !covered {
			return
		}
	}
	for row := range original {
		if starts[row.StartPc] && !expected[row] {
			return
		}
	}
	n.tryDispatchLowered = w
}
