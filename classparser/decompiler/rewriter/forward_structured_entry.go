package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A completed decision keeps its original incoming boundary even though its
// wrapper is not an original instruction. Only forward-region boundaries use
// this witness: it cannot justify moving effects or invent an opcode origin.
type forwardEntry struct {
	statement *statements.IfStatement
	pc        int
}

func rememberForwardEntry(m *RewriteManager, original, replacement *core.Node) {
	if m == nil || original == nil || replacement == nil || !original.HasOriginPC || original.OriginPC < 0 || original.OriginPC > 65535 || original.HideNext != nil || original.IsCatchStart || original.IsTryCatch || original.HasProtectedRange || replacement.HasOriginPC || len(m.forwardEntries) >= 4096 {
		return
	}
	raw, ok := original.Statement.(*statements.ConditionStatement)
	if !ok || raw == nil || raw.Condition == nil {
		return
	}
	completed, ok := replacement.Statement.(*statements.IfStatement)
	if !ok || completed == nil || completed.Condition == nil {
		return
	}
	if m.forwardEntries == nil {
		m.forwardEntries = map[*core.Node]forwardEntry{}
	}
	m.forwardEntries[replacement] = forwardEntry{statement: completed, pc: original.OriginPC}
}

func forwardEntryPC(m *RewriteManager, n *core.Node) (int, bool) {
	if n == nil {
		return 0, false
	}
	if n.HasOriginPC {
		return n.OriginPC, n.OriginPC >= 0 && n.OriginPC <= 65535
	}
	if m == nil || n.HideNext != nil || n.IsCatchStart || n.IsTryCatch || n.HasProtectedRange {
		return 0, false
	}
	proof, ok := m.forwardEntries[n]
	current, typed := n.Statement.(*statements.IfStatement)
	if !ok || !typed || current == nil || current != proof.statement || current.Condition == nil || proof.pc < 0 || proof.pc > 65535 {
		return 0, false
	}
	return proof.pc, true
}
