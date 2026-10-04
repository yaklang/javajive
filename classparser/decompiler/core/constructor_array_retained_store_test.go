package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestConstructorArrayEntryRetainsOriginalFieldStores(t *testing.T) {
	for _, variant := range []string{"original", "two stores", "foreign receiver", "opaque receiver", "stack receiver", "missing source PC", "missing opcode", "wrong opcode", "different PC", "try entry", "catch entry", "alternate successor", "cycle", "bound"} {
		t.Run(variant, func(t *testing.T) {
			receiver := &values.JavaRef{IsThis: true}
			field := values.NewRefMember(receiver, "independentCapture", types.NewJavaClass("UnrelatedOwner"))
			assign := statements.NewAssignStatement(field, values.JavaNull, false)
			assign.HasOriginPC = true
			assign.OriginPC = 5
			store := NewNode(assign)
			store.Id = 1
			array := NewNode(&statements.AssignStatement{LeftValue: &values.JavaRef{}})
			array.Id = 2
			store.AddNext(array)
			origins := map[int]*OpCode{1: {Instr: InstrInfos[OP_PUTFIELD], CurrentOffset: 5}}
			switch variant {
			case "two stores":
				copy := *assign
				second := NewNode(&copy)
				second.Id = 3
				store.ReplaceNext(array, second)
				second.AddNext(array)
				origins[3] = &OpCode{Instr: InstrInfos[OP_PUTFIELD], CurrentOffset: 5}
			case "foreign receiver":
				receiver.IsThis = false
			case "opaque receiver":
				receiver.CustomValue = &values.CustomValue{}
			case "stack receiver":
				receiver.StackVar = values.JavaNull
			case "missing source PC":
				assign.HasOriginPC = false
			case "missing opcode":
				delete(origins, 1)
			case "wrong opcode":
				origins[1].Instr = InstrInfos[OP_PUTSTATIC]
			case "different PC":
				origins[1].CurrentOffset = 6
			case "try entry":
				store.IsTryCatch = true
			case "catch entry":
				store.IsCatchStart = true
			case "alternate successor":
				store.AddNext(NewNode(&statements.ReturnStatement{}))
			case "cycle":
				store.ReplaceNext(array, store)
			case "bound":
				current := store
				for i := 0; i < 256; i++ {
					copy := *assign
					next := NewNode(&copy)
					next.Id = 10 + i
					current.ReplaceNext(array, next)
					next.AddNext(array)
					origins[next.Id] = &OpCode{Instr: InstrInfos[OP_PUTFIELD], CurrentOffset: 5}
					current = next
				}
			}
			d := &Decompiler{RootNode: store}
			oldNext := store.Next[0]
			entry, prefix, conditions, ok := d.constructorArrayEntryAfterRetainedStores(origins)
			want := variant == "original" || variant == "two stores"
			if ok != want {
				t.Fatalf("retained entry=%v", ok)
			}
			if ok && (entry != array || !prefix[store] || len(conditions) != 0 || d.RootNode != store || store.Next[0] != oldNext) {
				t.Fatal("field store changed or removed by array discovery")
			}
		})
	}
}
