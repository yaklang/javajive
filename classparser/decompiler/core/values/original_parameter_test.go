package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalParameterSlotDoesNotFollowSourceNamesOrReplacementValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"original", true}, {"renamed", true}, {"second registration cannot rebind", true},
		{"unwitnessed", false}, {"parameter flag removed", false}, {"this", false}, {"same-typed replacement seed", false}, {"custom", false}, {"stack temporary", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ := types.NewJavaClass("example.Enum")
			seed := NewJavaLiteral("seed", typ)
			ref := NewJavaRef(utils.NewRootVariableId(), seed, typ)
			ref.IsParam = true
			if tc.name != "unwitnessed" {
				ref.MarkOriginalParameter(3)
			}
			switch tc.name {
			case "renamed":
				ref.Id = utils.NewRootVariableId()
			case "second registration cannot rebind":
				ref.MarkOriginalParameter(7)
			case "parameter flag removed":
				ref.IsParam = false
			case "this":
				ref.IsThis = true
			case "same-typed replacement seed":
				ref.Val = NewJavaLiteral("seed", typ)
			case "custom":
				ref.CustomValue = &CustomValue{}
			case "stack temporary":
				ref.StackVar = seed
			}
			slot, known := ref.OriginalParameterSlot()
			if known != tc.valid || known && slot != 3 {
				t.Fatalf("slot=%d known=%v", slot, known)
			}
		})
	}
	var missing *JavaRef
	if _, known := missing.OriginalParameterSlot(); known {
		t.Fatal("nil ref accepted")
	}
}
