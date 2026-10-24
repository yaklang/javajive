package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalDeclarationAliasRequiresExplicitSourceCloneAndStableSeed(t *testing.T) {
	for _, variant := range []string{"original", "source clone", "renamed cohort", "shallow copy", "copied ID", "changed ID", "changed UID", "changed seed", "changed source seed", "parameter", "this", "opaque", "unsealed", "clone bound", "nil"} {
		t.Run(variant, func(t *testing.T) {
			seed := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("probe.Value"))
			ref := NewJavaRef(utils.NewRootVariableId(), seed, seed.Type())
			ref.VarUid = "actual decoded local"
			ref.MarkOriginalLocalDeclaration(7, 2, seed)
			other := ref.CloneForSourceDeclaration()
			switch variant {
			case "original":
				other = ref
			case "renamed cohort":
				id := utils.NewRootVariableId()
				ref.Id, other.Id = id, id
			case "shallow copy":
				copy := *ref
				other = &copy
			case "copied ID":
				other = NewJavaRef(ref.Id, seed, seed.Type())
				other.VarUid = ref.VarUid
				other.MarkOriginalLocalDeclaration(7, 2, seed)
			case "changed ID":
				other.Id = utils.NewRootVariableId()
			case "changed UID":
				other.VarUid = "foreign"
			case "changed seed":
				other.Val = JavaNull
			case "changed source seed":
				ref.Val = JavaNull
			case "parameter":
				other.IsParam = true
			case "this":
				other.IsThis = true
			case "opaque":
				other.CustomValue = &CustomValue{}
			case "unsealed":
				ref = seed
				other = seed.CloneForSourceDeclaration()
			case "clone bound":
				for i := 0; i < 64; i++ {
					other = ref.CloneForSourceDeclaration()
				}
				if len(ref.originalDeclarationAliases.members) != 64 {
					t.Fatal("source clone cohort escaped bound")
				}
			case "nil":
				other = nil
			}
			positive := variant == "original" || variant == "source clone" || variant == "renamed cohort"
			if ref.SameOriginalLocalDeclaration(other) != positive || other.SameOriginalLocalDeclaration(ref) != positive {
				t.Fatalf("%s: declaration clone certificate differs from %v", variant, positive)
			}
		})
	}
}
