package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNullAdoptionDoesNotTreatObjectOverwriteAsNullDefinition(t *testing.T) {
	// The null arm returns. The sibling stores a monitor object using a ref the
	// legacy simulator reused, then overwrites the slot with a concrete object.
	entry, nullStore, monitorStore, concreteStore := op(OP_NOP, 0), op(OP_ASTORE_1, 1), op(OP_ASTORE_1, 2), op(OP_ASTORE_1, 3)
	nullStore.Source = []*OpCode{entry}
	monitorStore.Source = []*OpCode{entry}
	concreteStore.Source = []*OpCode{monitorStore}
	null := values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
	ref := values.NewJavaRef(nil, null, types.NewJavaClass("java.lang.Object"))
	d := &Decompiler{opCodes: []*OpCode{entry, nullStore, monitorStore, concreteStore},
		opcodeIdToRef:  map[*OpCode][][2]any{nullStore: {{ref, true}}, monitorStore: {{ref, false}}},
		slotStoreValue: map[*OpCode]values.JavaValue{nullStore: null, monitorStore: values.NewRefMember(ref, "lock", types.NewJavaClass("java.lang.Object"))},
	}
	if d.nullInitDefDominates(concreteStore, 1, ref) {
		t.Fatal("non-null monitor store cannot justify narrowing the earlier null return local")
	}
	concreteStore.Source = []*OpCode{nullStore}
	if !d.nullInitDefDominates(concreteStore, 1, ref) {
		t.Fatal("genuine null definition should still permit adoption")
	}
}
