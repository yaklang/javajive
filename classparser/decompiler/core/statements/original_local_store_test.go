package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLocalStoreRetainsEachLogicalBindingAndRHS(t *testing.T) {
	for _, variant := range []string{"original", "null sentinel", "typed null literal", "source rename", "source ID placement", "statement clone", "repeated mark", "unwitnessed", "different RHS", "reset slot RHS", "different ref", "same ID other ref", "wrong UID", "missing ID", "parameter", "receiver", "custom", "stack alias", "changed PC", "no origin", "array store", "deep forwarding"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			var seed values.JavaValue = values.NewJavaLiteral(int64(17), typ)
			if variant == "null sentinel" {
				seed = values.JavaNull
			} else if variant == "typed null literal" {
				seed = values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			slot := values.NewSlotValue(seed, typ)
			a := NewAssignStatement(ref, slot, false)
			a.OriginPC, a.HasOriginPC = 11, true
			if variant != "unwitnessed" {
				a.MarkOriginalLocalStore(11, 3)
			}
			want := variant == "original" || variant == "null sentinel" || variant == "typed null literal" || variant == "source rename" || variant == "source ID placement" || variant == "statement clone" || variant == "repeated mark"
			switch variant {
			case "source rename":
				ref.Id.SetName("other")
			case "source ID placement":
				ref.Id = utils.NewRootVariableId()
			case "statement clone":
				copy := *a
				a = &copy
			case "repeated mark":
				a.MarkOriginalLocalStore(12, 4)
			case "different RHS":
				a.JavaValue = values.NewJavaLiteral(int64(17), typ)
			case "reset slot RHS":
				slot.ResetValue(values.NewJavaLiteral(int64(17), typ))
			case "different ref":
				a.LeftValue = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "same ID other ref":
				a.LeftValue = values.NewJavaRef(ref.Id, nil, typ)
			case "wrong UID":
				ref.VarUid += "other"
			case "missing ID":
				ref.Id = nil
			case "parameter":
				ref.IsParam = true
			case "receiver":
				ref.IsThis = true
			case "custom":
				ref.CustomValue = &values.CustomValue{}
			case "stack alias":
				ref.StackVar = seed
			case "changed PC":
				a.OriginPC++
			case "no origin":
				a.HasOriginPC = false
			case "array store":
				a.ArrayMember = &values.JavaArrayMember{}
			case "deep forwarding":
				for i := 0; i < 33; i++ {
					a.JavaValue = values.NewSlotValue(a.JavaValue, typ)
				}
			}
			pc, local, known := a.OriginalLocalStore()
			if known != want || known && (pc != 11 || local != 3) {
				t.Fatalf("STORE=%d/%d/%v want=%v", pc, local, known, want)
			}
		})
	}
}
