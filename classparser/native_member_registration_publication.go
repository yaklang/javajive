package javaclassparser

// A declaration scheduler changes the enclosing packet while preserving every
// executable/initializer atom. Its successful search supplies the exact new
// packet; source text alone cannot establish a lexical class or authorize an
// unproved reorder. Receipts from rejected backtracking paths never publish.
type nativeMemberRegistrationReceipt struct {
	object        *ClassObject
	before, after string
}

type nativeMemberRegistrationPublication struct {
	sourceHash [32]byte
	members    map[string]nativeMemberRegistrationReceipt
}

func (p *nativeMemberFamily) registrationMemberPacket(root, member *ClassObject, before string, finalHash [32]byte) (string, bool) {
	if p == nil || p.failed || root == nil || member == nil || p.owner != root.GetClassName() || p.lexicalObjects[p.owner] != root || p.registrationPublication == nil || p.registrationPublication.sourceHash != finalHash {
		return "", false
	}
	child := p.children[member.GetClassName()]
	if child == nil || child.object != member || child.owner != p.owner {
		return "", false
	}
	receipt, ok := p.registrationPublication.members[member.GetClassName()]
	if !ok || receipt.object != member || receipt.before != before || receipt.after == "" {
		return "", false
	}
	return receipt.after, true
}
