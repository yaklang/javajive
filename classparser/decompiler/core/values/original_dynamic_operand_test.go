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

func TestOriginalDynamicOperandDeclarationRequiresTheSamePrivateWitness(t *testing.T) {
	for _, variant := range []string{"original", "logical clone", "renamed", "equal public fields", "foreign same factory", "split ID", "changed seed", "changed UID", "unwitnessed", "nil"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			seed := NewJavaLiteral(int64(17), typ)
			snapshot := NewJavaRef(utils.NewRootVariableId(), seed, typ)
			snapshot.MarkOriginalDynamicOperand(12, 2, seed)
			snapshot.MarkOriginalDynamicOperandDeclaration(12, seed)
			copy := *snapshot
			declaration := &copy
			switch variant {
			case "original":
				declaration = snapshot
			case "renamed":
				snapshot.Id.SetName("renamed")
			case "equal public fields", "foreign same factory":
				declaration = NewJavaRef(snapshot.Id, seed, typ)
				declaration.VarUid = snapshot.VarUid
				if variant == "foreign same factory" {
					declaration.MarkOriginalDynamicOperand(12, 2, seed)
					declaration.MarkOriginalDynamicOperandDeclaration(12, seed)
				}
			case "split ID":
				declaration.Id = utils.NewRootVariableId()
			case "changed seed":
				declaration.Val = NewJavaLiteral(int64(17), typ)
			case "changed UID":
				declaration.VarUid += "other"
			case "unwitnessed":
				declaration.originalDynamicOperand = nil
			case "nil":
				declaration = nil
			}
			want := variant == "original" || variant == "logical clone" || variant == "renamed"
			if declaration.OriginalDynamicOperandDeclarationOf(snapshot, seed) != want {
				t.Fatal("source declaration borrowed another snapshot")
			}
		})
	}
}
