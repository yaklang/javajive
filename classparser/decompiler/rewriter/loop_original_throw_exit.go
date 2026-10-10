package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A generated loop has a normal continuation only for a normally completing
// exit. Keep an original, privately owned ATHROW in its conditional arm rather
// than replacing it with a break and lifting it after the loop. This retains
// typed operand/origin identity for later source reconstruction. Printed throw
// text, shared entries, exceptional entries, and nonlocal transfers cannot
// establish this capability. Region ownership is checked independently.
func originalLoopThrowLeaf(n *core.Node) bool {
	if n == nil || n.IsCatchStart || !n.HasOriginPC || n.OriginPC < 0 {
		return false
	}
	st, ok := n.Statement.(*statements.CustomStatement)
	if !ok || st == nil || st.ThrownValue == nil || !st.HasOriginPC || st.OriginPC != n.OriginPC || st.HasSourceTransfer() || st.LoopTransferKind != "" {
		return false
	}
	for _, next := range n.Next {
		if !IsEndNode(next) {
			return false
		}
	}
	return true
}
