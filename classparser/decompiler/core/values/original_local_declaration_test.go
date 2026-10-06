package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalLocalDeclarationRequiresImmutableStoreAndSeed(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "second registration", "unregistered", "parameter", "this", "replaced seed", "different source seed", "custom", "stack", "bad PC", "bad slot"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			seed := NewJavaLiteral("same", typ)
			ref := NewJavaRef(utils.NewRootVariableId(), seed, typ)
			pc, slot := 9, 4
			if variant == "bad PC" {
				pc = -1
			}
			if variant == "bad slot" {
				slot = -1
			}
			if variant != "unregistered" {
				ref.MarkOriginalLocalDeclaration(pc, slot, seed)
			}
			source := JavaValue(seed)
			switch variant {
			case "renamed":
				ref.Id = utils.NewRootVariableId()
			case "second registration":
				ref.MarkOriginalLocalDeclaration(19, 5, seed)
			case "parameter":
				ref.IsParam = true
			case "this":
				ref.IsThis = true
			case "replaced seed":
				ref.Val = NewJavaLiteral("same", typ)
			case "different source seed":
				source = NewJavaLiteral("same", typ)
			case "custom":
				ref.CustomValue = &CustomValue{}
			case "stack":
				ref.StackVar = seed
			}
			gotPC, gotSlot, known := ref.OriginalLocalDeclaration(source)
			want := variant == "original" || variant == "renamed" || variant == "second registration"
			if known != want || known && (gotPC != 9 || gotSlot != 4) {
				t.Fatalf("store %d/%d known %v", gotPC, gotSlot, known)
			}
		})
	}
	var missing *JavaRef
	if _, _, known := missing.OriginalLocalDeclaration(nil); known {
		t.Fatal("nil local")
	}
}
