package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
)

// A physical PUTFIELD fixes both evaluated operands, independently of source
// naming and local placement. Forwarding slots may retain the same seed; a
// different receiver/RHS cannot borrow this store's identity or abrupt point.
type originalInstanceStore struct {
	pc                        int
	owner, member, descriptor string
	target                    *values.RefMember
	receiver, rhs             values.JavaValue
}

func (a *AssignStatement) MarkOriginalInstanceFieldStore(member *values.JavaClassMember, pc int) {
	if a == nil || a.originalInstanceStore != nil || member == nil || !a.HasOriginPC || a.OriginPC != pc || pc < 0 || pc > 65535 || a.ArrayMember != nil || a.IsDeclare {
		return
	}
	field, ok := a.LeftValue.(*values.RefMember)
	if !ok || field == nil || field.Member != member.Member {
		return
	}
	receiver, rhs := originalStoreSeed(field.Object), originalStoreSeed(a.JavaValue)
	if receiver == nil || rhs == nil {
		return
	}
	a.originalInstanceStore = &originalInstanceStore{pc, strings.ReplaceAll(member.Name, ".", "/"), member.Member, member.Description, field, receiver, rhs}
}

func (a *AssignStatement) OriginalInstanceFieldStore() (pc int, owner, member, descriptor string, known bool) {
	if a == nil || a.originalInstanceStore == nil || !a.HasOriginPC || a.ArrayMember != nil || a.IsDeclare {
		return
	}
	w := a.originalInstanceStore
	field, ok := a.LeftValue.(*values.RefMember)
	if !ok || field != w.target || field.Member != w.member || a.OriginPC != w.pc || originalStoreSeed(field.Object) != w.receiver || originalStoreSeed(a.JavaValue) != w.rhs {
		return
	}
	return w.pc, w.owner, w.member, w.descriptor, true
}
