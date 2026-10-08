package statements

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Each physical STORE keeps its own RHS identity. A multi-definition local
// cannot borrow the first declaration's initializer as evidence for all arms.
type originalLocalStore struct {
	pc, slot int
	ref      *values.JavaRef
	uid      string
	seed     values.JavaValue
}

func originalStoreSeed(v values.JavaValue) values.JavaValue {
	for depth := 0; depth < 32; depth++ {
		if v == nil || !reflect.TypeOf(v).Comparable() {
			return nil
		}
		if v == values.JavaNull {
			return v
		}
		if reflect.ValueOf(v).Kind() != reflect.Ptr || reflect.ValueOf(v).IsNil() {
			return nil
		}
		if slot, ok := v.(*values.SlotValue); ok {
			v = slot.GetValue()
			continue
		}
		return v
	}
	return nil
}

// Called only when the decoder emits an actual local STORE assignment node.
func (a *AssignStatement) MarkOriginalLocalStore(pc, slot int) {
	if a == nil || a.originalLocalStore != nil || !a.HasOriginPC || a.OriginPC != pc || pc < 0 || pc > 65535 || slot < 0 || slot > 65535 || a.ArrayMember != nil {
		return
	}
	ref, ok := a.LeftValue.(*values.JavaRef)
	seed := originalStoreSeed(a.JavaValue)
	if !ok || ref == nil || ref.Id == nil || ref.VarUid == "" || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || seed == nil {
		return
	}
	a.originalLocalStore = &originalLocalStore{pc: pc, slot: slot, ref: ref, uid: ref.VarUid, seed: seed}
}

func (a *AssignStatement) OriginalLocalStore() (pc, slot int, known bool) {
	if a == nil || a.originalLocalStore == nil || a.ArrayMember != nil || !a.HasOriginPC {
		return 0, 0, false
	}
	w := a.originalLocalStore
	ref, ok := a.LeftValue.(*values.JavaRef)
	seed := originalStoreSeed(a.JavaValue)
	// Declaration placement can replace Id while preserving this exact logical
	// ref and immutable VarUid. Names and Id equality confer no STORE witness.
	if !ok || ref != w.ref || ref.Id == nil || ref.VarUid != w.uid || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || a.OriginPC != w.pc || seed == nil || seed != w.seed {
		return 0, 0, false
	}
	return w.pc, w.slot, true
}
