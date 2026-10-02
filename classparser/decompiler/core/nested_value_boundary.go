package core

// closedNestedValueRoot finds the single entry of an already discovered value
// diamond. It is a routing boundary, not permission to fold its producers.
// Walking backwards through its two incoming value edges without contracting
// this boundary loses dominating short-circuit guards. Require every original
// incoming edge to be inside one forward, single-entry region in one original exception domain.
func (d *Decompiler) closedNestedValueRoot(merge *OpCode, candidates []*OpCode) *OpCode {
	if len(candidates) > 128 {
		return nil
	}
	var best *OpCode
	for _, root := range candidates {
		if root == nil || root.Instr == nil || len(root.Target) != 2 || merge == nil || merge.Instr == nil || merge.IsCatch || merge.IsTryCatchParent || len(merge.Source) < 2 {
			continue
		}
		switch root.Instr.OpCode {
		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
		default:
			continue
		}
		domain := d.handlersAt(root)
		if !sameHandlerCoverage(domain, d.handlersAt(merge)) {
			continue
		}
		region := map[*OpCode]bool{}
		pending := []*OpCode{root}
		valid := true
		for len(pending) > 0 && valid {
			n := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if n == merge || region[n] {
				continue
			}
			if n == nil || n.Instr == nil || n.IsCatch || n.IsTryCatchParent || !sameHandlerCoverage(domain, d.handlersAt(n)) || n.CurrentOffset < root.CurrentOffset || n.CurrentOffset >= merge.CurrentOffset || len(n.Target) == 0 || len(region) >= 512 {
				valid = false
				break
			}
			region[n] = true
			for _, next := range n.Target {
				if next == nil || next.CurrentOffset <= n.CurrentOffset {
					valid = false
					break
				}
				pending = append(pending, next)
			}
		}
		if !valid {
			continue
		}
		for n := range region {
			if n == root {
				continue
			}
			for _, p := range n.Source {
				if !region[p] {
					valid = false
					break
				}
			}
		}
		for _, p := range merge.Source {
			if !region[p] {
				valid = false
				break
			}
		}
		if valid && (best == nil || root.CurrentOffset < best.CurrentOffset) {
			best = root
		}
	}
	return best
}
