package statements

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// A parameter declaration denotes mutable storage, whereas its entry value is
// one particular definition. Keep a separate STORE witness: a local declaration
// witness must never be borrowed to certify assignment to a parameter.
type originalParameterStore struct {
	pc, slot int
	ref      *values.JavaRef
	uid      string
	seed     values.JavaValue
}

func (a *AssignStatement) MarkOriginalParameterStore(pc, slot int) {
	if a == nil || a.originalParameterStore != nil || !a.HasOriginPC || a.OriginPC != pc || pc < 0 || pc > 65535 || a.ArrayMember != nil || a.IsDeclare || a.IsFirst {
		return
	}
	ref, ok := a.LeftValue.(*values.JavaRef)
	seed := originalStoreSeed(a.JavaValue)
	if !ok || ref == nil || ref.Id == nil || ref.VarUid == "" || seed == nil {
		return
	}
	original, known := ref.OriginalParameterSlot()
	if !known || original != slot {
		return
	}
	a.originalParameterStore = &originalParameterStore{pc: pc, slot: slot, ref: ref, uid: ref.VarUid, seed: seed}
}

func (a *AssignStatement) OriginalParameterStore() (pc, slot int, known bool) {
	if a == nil || a.originalParameterStore == nil || !a.HasOriginPC || a.ArrayMember != nil || a.IsDeclare || a.IsFirst {
		return 0, 0, false
	}
	w := a.originalParameterStore
	ref, ok := a.LeftValue.(*values.JavaRef)
	seed := originalStoreSeed(a.JavaValue)
	if !ok || ref != w.ref || ref.Id == nil || ref.VarUid != w.uid || a.OriginPC != w.pc || seed == nil || seed != w.seed {
		return 0, 0, false
	}
	original, sealed := ref.OriginalParameterSlot()
	if !sealed || original != w.slot {
		return 0, 0, false
	}
	return w.pc, w.slot, true
}
