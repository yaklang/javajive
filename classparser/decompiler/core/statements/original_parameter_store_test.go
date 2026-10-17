package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalParameterStoreRequiresDeclarationAndExactAssignment(t *testing.T) {
	for _, variant := range []string{"original", "source rename", "cannot rebind", "unwitnessed", "wrong slot", "same public fields", "new RHS", "new UID", "new parameter seed", "not parameter", "receiver", "stack", "custom", "new PC", "no PC", "declaration", "first assignment", "array store", "nil statement"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("MutableState")
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			ref.IsParam = true
			ref.MarkOriginalParameter(3)
			rhs := values.NewJavaLiteral("next", typ)
			a := NewAssignStatement(ref, rhs, false)
			a.OriginPC, a.HasOriginPC = 11, true
			if variant != "unwitnessed" {
				slot := 3
				if variant == "wrong slot" {
					slot = 4
				}
				a.MarkOriginalParameterStore(11, slot)
			}
			switch variant {
			case "source rename":
				ref.Id = utils.NewRootVariableId()
			case "cannot rebind":
				a.MarkOriginalParameterStore(17, 4)
			case "same public fields":
				other := values.NewJavaRef(ref.Id, nil, typ)
				other.IsParam, other.VarUid = true, ref.VarUid
				other.MarkOriginalParameter(3)
				a.LeftValue = other
			case "new RHS":
				a.JavaValue = values.NewJavaLiteral("next", typ)
			case "new UID":
				ref.VarUid += "other"
			case "new parameter seed":
				ref.Val = rhs
			case "not parameter":
				ref.IsParam = false
			case "receiver":
				ref.IsThis = true
			case "stack":
				ref.StackVar = rhs
			case "custom":
				ref.CustomValue = &values.CustomValue{}
			case "new PC":
				a.OriginPC = 17
			case "no PC":
				a.HasOriginPC = false
			case "declaration":
				a.IsDeclare = true
			case "first assignment":
				a.IsFirst = true
			case "array store":
				a.ArrayMember = &values.JavaArrayMember{}
			case "nil statement":
				a = nil
			}
			pc, slot, known := a.OriginalParameterStore()
			want := variant == "original" || variant == "source rename" || variant == "cannot rebind"
			if known != want || known && (pc != 11 || slot != 3) {
				t.Fatalf("pc=%d slot=%d known=%v want=%v", pc, slot, known, want)
			}
			if a != nil {
				if _, _, known := a.OriginalLocalStore(); known {
					t.Fatal("parameter STORE borrowed local-declaration witness")
				}
			}
		})
	}
}
