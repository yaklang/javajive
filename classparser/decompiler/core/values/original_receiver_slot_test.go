package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalReceiverRequiresDecodedRoleAndStableDescriptorSeed(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "restamped", "no parameter witness", "wrong slot", "static slot zero", "missing receiver witness", "role removed", "parameter removed", "seed replaced", "custom", "stack"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("Owner")
			seed := NewJavaLiteral("seed", typ)
			ref := NewJavaRef(utils.NewRootVariableId(), seed, typ)
			ref.IsParam = true
			slot := 0
			if variant == "wrong slot" {
				slot = 1
			}
			if variant != "no parameter witness" {
				ref.MarkOriginalParameter(slot)
			}
			ref.IsThis = true
			if variant != "static slot zero" && variant != "missing receiver witness" {
				ref.MarkOriginalReceiver()
			}
			switch variant {
			case "renamed":
				ref.Id = utils.NewRootVariableId()
			case "restamped":
				ref.MarkOriginalParameter(3)
				ref.MarkOriginalReceiver()
			case "role removed":
				ref.IsThis = false
			case "parameter removed":
				ref.IsParam = false
			case "seed replaced":
				ref.Val = NewJavaLiteral("seed", typ)
			case "custom":
				ref.CustomValue = &CustomValue{}
			case "stack":
				ref.StackVar = seed
			}
			got, ok := ref.OriginalReceiverSlot()
			want := variant == "original" || variant == "renamed" || variant == "restamped"
			if ok != want || ok && got != 0 {
				t.Fatalf("receiver=(%d,%v), want known=%v", got, ok, want)
			}
			if _, known := ref.OriginalParameterSlot(); known {
				t.Fatal("implicit receiver relabeled as an explicit parameter")
			}
		})
	}
	var absent *JavaRef
	if _, ok := absent.OriginalReceiverSlot(); ok {
		t.Fatal("nil receiver accepted")
	}
}

func TestOriginalInstanceFieldReadCannotBorrowDifferentMemberOrPC(t *testing.T) {
	for _, variant := range []string{"original", "no witness", "wrong pc", "missing pc", "wrong owner", "wrong name", "wrong descriptor", "renamed field", "missing receiver", "restamped"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("Mode")
			member := NewJavaClassMember("Owner", "mode", "LMode;", typ)
			receiver := NewJavaRef(utils.NewRootVariableId(), JavaNull, types.NewJavaClass("Owner"))
			field := NewRefMember(receiver, "mode", typ)
			field.HasOriginPC = true
			field.OriginPC = 7
			if variant != "no witness" {
				field.MarkOriginalFieldRead(member, 7)
			}
			pc, owner, name, desc := 7, "Owner", "mode", "LMode;"
			switch variant {
			case "wrong pc":
				pc++
			case "missing pc":
				field.HasOriginPC = false
			case "wrong owner":
				owner = "Other"
			case "wrong name":
				name = "other"
			case "wrong descriptor":
				desc = "LOther;"
			case "renamed field":
				field.Member = "other"
			case "missing receiver":
				field.Object = nil
			case "restamped":
				field.MarkOriginalFieldRead(NewJavaClassMember("Other", "other", "LOther;", typ), 9)
			}
			got := field.OriginalInstanceFieldRead(pc, owner, name, desc)
			want := variant == "original" || variant == "restamped"
			if got != want {
				t.Fatalf("field certificate=%v want %v", got, want)
			}
		})
	}
}
