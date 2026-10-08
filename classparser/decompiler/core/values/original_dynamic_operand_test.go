package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalDynamicOperandWitnessRequiresOriginalSnapshotAndDeclaration(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "logical clone", "second registration", "no registration", "no declaration", "wrong declaration pc", "wrong declaration seed", "changed seed", "different queried seed", "foreign uid", "parameter", "receiver", "custom", "stack", "negative pc", "oversize pc", "negative position", "oversize position", "nil seed"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("Box")
			var seed JavaValue = NewJavaLiteral(nil, typ)
			if variant == "nil seed" {
				seed = nil
			}
			ref := NewJavaRef(utils.NewRootVariableId(), seed, typ)
			pc, index := 12, 2
			switch variant {
			case "negative pc":
				pc = -1
			case "oversize pc":
				pc = 65536
			case "negative position":
				index = -1
			case "oversize position":
				index = 64
			}
			if variant != "no registration" {
				ref.MarkOriginalDynamicOperand(pc, index, seed)
			}
			declPC, declSeed := 12, seed
			if variant == "wrong declaration pc" {
				declPC++
			}
			if variant == "wrong declaration seed" {
				declSeed = NewJavaLiteral(nil, typ)
			}
			if variant != "no declaration" {
				ref.MarkOriginalDynamicOperandDeclaration(declPC, declSeed)
			}
			queried := seed
			switch variant {
			case "renamed":
				ref.Id = utils.NewRootVariableId()
			case "logical clone":
				copy := *ref
				ref = &copy
			case "second registration":
				ref.MarkOriginalDynamicOperand(19, 1, seed)
			case "changed seed":
				ref.Val = NewJavaLiteral(nil, typ)
			case "different queried seed":
				queried = NewJavaLiteral(nil, typ)
			case "foreign uid":
				ref.VarUid += "foreign"
			case "parameter":
				ref.IsParam = true
			case "receiver":
				ref.IsThis = true
			case "custom":
				ref.CustomValue = &CustomValue{}
			case "stack":
				ref.StackVar = seed
			}
			location, position, known := ref.OriginalDynamicOperandWitness(queried)
			want := variant == "original" || variant == "renamed" || variant == "logical clone" || variant == "second registration"
			if known != want || known && (location != 12 || position != 2) {
				t.Fatalf("witness=%d/%d/%v want %v", location, position, known, want)
			}
		})
	}
}
