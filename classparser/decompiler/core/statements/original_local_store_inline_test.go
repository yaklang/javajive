package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLocalStoreInlineKeepsExactDefinitionAndConsumer(t *testing.T) {
	for _, change := range []string{"original", "slot wrappers", "missing store", "changed store PC", "changed consumer", "changed definition", "same public reference", "missing origin", "later definition", "bare declaration", "not first", "array", "parameter", "receiver", "inline reference", "opaque reference", "changed value", "changed history", "changed replacement"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			value := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			tmp := values.NewJavaRef(utils.NewRootVariableId(), value, typ)
			definition := NewAssignStatement(tmp, value, true)
			definition.OriginPC, definition.HasOriginPC = 7, true
			local := values.NewJavaRef(utils.NewRootVariableId(), tmp, typ)
			store := NewAssignStatement(local, tmp, true)
			store.OriginPC, store.HasOriginPC = 10, true
			if change != "missing store" {
				store.MarkOriginalLocalStore(10, 3)
			}
			original := *store
			var replacement values.JavaValue = value
			switch change {
			case "slot wrappers":
				definition.JavaValue = values.NewSlotValue(value, typ)
				store.JavaValue = values.NewSlotValue(tmp, typ)
				replacement = values.NewSlotValue(value, typ)
			case "changed store PC":
				store.OriginPC++
			case "changed consumer":
				store.JavaValue = value
			case "changed definition":
				definition.LeftValue = local
			case "same public reference":
				copy := *tmp
				definition.LeftValue = &copy
			case "missing origin":
				definition.HasOriginPC = false
			case "later definition":
				definition.OriginPC = 11
			case "bare declaration":
				definition.IsDeclare = true
			case "not first":
				definition.IsFirst = false
			case "array":
				definition.ArrayMember = &values.JavaArrayMember{}
			case "parameter":
				tmp.IsParam = true
			case "receiver":
				tmp.IsThis = true
			case "inline reference":
				tmp.StackVar = value
			case "opaque reference":
				tmp.CustomValue = &values.CustomValue{}
			case "changed value":
				definition.JavaValue = values.NewJavaRef(value.Id, nil, typ)
			case "changed history":
				tmp.Val = values.NewJavaRef(value.Id, nil, typ)
			case "changed replacement":
				replacement = values.NewJavaRef(value.Id, nil, typ)
			}
			before := *store
			want := change == "original" || change == "slot wrappers"
			if got := store.ProjectOriginalLocalStoreInline(definition, tmp, replacement); got != want {
				t.Fatalf("STORE projection=%v want=%v", got, want)
			}
			if store.JavaValue != before.JavaValue || store.LeftValue != before.LeftValue || store.OriginPC != before.OriginPC || store.IsFirst != before.IsFirst {
				t.Fatal("receipt transport changed an evaluation or declaration")
			}
			if !want {
				return
			}
			store.JavaValue = replacement // The already proved decoder substitution.
			pc, slot, known := store.OriginalLocalStore()
			if !known || pc != 10 || slot != 3 {
				t.Fatal("actual STORE identity was lost after the approved substitution")
			}
			if _, _, known := original.OriginalLocalStore(); !known {
				t.Fatal("a different view's original receipt was mutated")
			}
			store.JavaValue = values.NewJavaRef(value.Id, nil, typ)
			if _, _, known := store.OriginalLocalStore(); known {
				t.Fatal("a same-typed replacement borrowed the transported STORE receipt")
			}
		})
	}
}
