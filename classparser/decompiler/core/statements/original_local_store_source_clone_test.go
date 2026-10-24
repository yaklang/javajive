package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLocalStoreKeepsNamedSourceCloneWithoutInstallingHistoricalInitializer(t *testing.T) {
	for _, change := range []string{"original", "source rename", "source placement", "clone of clone", "other history", "different RHS", "shallow copy", "same public fields", "different source cohort", "changed ID", "changed UID", "changed original UID", "parameter", "receiver", "custom", "inline", "clone bound"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			seed := values.NewJavaLiteral(int64(17), typ)
			a := NewAssignStatement(ref, seed, true)
			a.OriginPC, a.HasOriginPC = 11, true
			a.MarkOriginalLocalStore(11, 3)
			clone := ref.CloneForSourceDeclaration()
			a.LeftValue = clone
			switch change {
			case "source rename":
				ref.Id.SetName("retained")
			case "source placement":
				ref.Id = utils.NewRootVariableId()
				clone.Id = ref.Id
			case "clone of clone":
				clone = clone.CloneForSourceDeclaration()
				a.LeftValue = clone
			case "other history":
				clone.Val = values.NewJavaLiteral(int64(99), typ)
			case "different RHS":
				a.JavaValue = values.NewJavaLiteral(int64(17), typ)
			case "shallow copy":
				copy := *ref
				a.LeftValue = &copy
			case "same public fields", "different source cohort":
				other := values.NewJavaRef(ref.Id, nil, typ)
				other.VarUid = ref.VarUid
				if change == "different source cohort" {
					other = other.CloneForSourceDeclaration()
				}
				a.LeftValue = other
			case "changed ID":
				clone.Id = utils.NewRootVariableId()
			case "changed UID":
				clone.VarUid += "changed"
			case "changed original UID":
				ref.VarUid += "changed"
			case "parameter":
				clone.IsParam = true
			case "receiver":
				clone.IsThis = true
			case "custom":
				clone.CustomValue = &values.CustomValue{}
			case "inline":
				clone.StackVar = seed
			case "clone bound":
				for i := 0; i < 64; i++ {
					clone = ref.CloneForSourceDeclaration()
				}
				a.LeftValue = clone
			}
			want := change == "original" || change == "source rename" || change == "source placement" || change == "clone of clone" || change == "other history"
			pc, slot, known := a.OriginalLocalStore()
			if known != want || known && (pc != 11 || slot != 3) {
				t.Fatalf("named STORE=%d/%d/%v want=%v", pc, slot, known, want)
			}
			if ref.Val != nil || clone.Val != nil && change != "other history" {
				t.Fatal("source cloning installed a historical initializer")
			}
			if _, _, known := ref.OriginalLocalDeclaration(seed); known {
				t.Fatal("source target identity acquired a value-definition witness")
			}
		})
	}
}
