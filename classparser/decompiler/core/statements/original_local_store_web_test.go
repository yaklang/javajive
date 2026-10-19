package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLocalStoreKeepsItsRHSIndependentOfSealedDefinitionWebHistory(t *testing.T) {
	for _, change := range []string{"original target", "original web member", "source rename", "source placement", "other history", "shallow copy", "same public fields", "foreign web", "changed ID", "changed UID", "changed original UID", "missing ID", "parameter", "receiver", "custom", "inline", "different RHS", "changed PC", "lost origin"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			id := utils.NewRootVariableId()
			first, second := values.NewJavaRef(id, nil, typ), values.NewJavaRef(id, nil, typ)
			second.VarUid = first.VarUid
			values.MarkOriginalLocalWeb([]*values.JavaRef{first, second})
			seed := values.NewJavaLiteral(int64(17), typ)
			a := NewAssignStatement(first, seed, false)
			a.OriginPC, a.HasOriginPC = 11, true
			a.MarkOriginalLocalStore(11, 3)
			a.LeftValue = second
			switch change {
			case "original target":
				a.LeftValue = first
			case "source rename":
				id.SetName("retained")
			case "source placement":
				first.Id = utils.NewRootVariableId()
				second.Id = first.Id
			case "other history":
				second.Val = values.NewJavaLiteral(int64(99), typ)
			case "shallow copy":
				copy := *second
				a.LeftValue = &copy
			case "same public fields":
				other := values.NewJavaRef(id, nil, typ)
				other.VarUid = first.VarUid
				a.LeftValue = other
			case "foreign web":
				other := values.NewJavaRef(id, nil, typ)
				other.VarUid = first.VarUid
				values.MarkOriginalLocalWeb([]*values.JavaRef{second, other})
			case "changed ID":
				second.Id = utils.NewRootVariableId()
			case "changed UID":
				second.VarUid += "changed"
			case "changed original UID":
				first.VarUid += "changed"
			case "missing ID":
				second.Id = nil
			case "parameter":
				second.IsParam = true
			case "receiver":
				second.IsThis = true
			case "custom":
				second.CustomValue = &values.CustomValue{}
			case "inline":
				second.StackVar = seed
			case "different RHS":
				a.JavaValue = values.NewJavaLiteral(int64(17), typ)
			case "changed PC":
				a.OriginPC++
			case "lost origin":
				a.HasOriginPC = false
			}
			want := change == "original target" || change == "original web member" || change == "source rename" || change == "source placement" || change == "other history"
			pc, slot, known := a.OriginalLocalStore()
			if known != want || known && (pc != 11 || slot != 3) {
				t.Fatalf("actual STORE=%d/%d/%v want=%v", pc, slot, known, want)
			}
		})
	}
}
