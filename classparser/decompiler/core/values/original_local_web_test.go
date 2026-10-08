package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLocalWebRequiresActualSealedMembersAndCurrentBinding(t *testing.T) {
	for _, variant := range []string{"original", "rename", "shared source ID change", "same public fields", "foreign same public fields", "no seal", "duplicate members", "nil member", "empty UID", "different UID at seal", "different ID at seal", "changed UID", "changed ID", "nil ID", "parameter at seal", "parameter after seal", "receiver", "custom", "stack", "different web", "clone", "oversize"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("Base")
			id := utils.NewRootVariableId()
			first, second := NewJavaRef(id, nil, typ), NewJavaRef(id, nil, typ)
			second.VarUid = first.VarUid
			members := []*JavaRef{first, second}
			switch variant {
			case "duplicate members":
				members = append(members, first)
			case "nil member":
				members = append(members, nil)
			case "empty UID":
				first.VarUid, second.VarUid = "", ""
			case "different UID at seal":
				second.VarUid += "other"
			case "different ID at seal":
				second.Id = utils.NewRootVariableId()
			case "parameter at seal":
				second.IsParam = true
			case "oversize":
				for len(members) < 65 {
					r := NewJavaRef(id, nil, typ)
					r.VarUid = first.VarUid
					members = append(members, r)
				}
			}
			if variant != "no seal" {
				MarkOriginalLocalWeb(members)
			}
			switch variant {
			case "rename":
				id.SetName("renamed")
			case "shared source ID change":
				first.Id = utils.NewRootVariableId()
				second.Id = first.Id
			case "same public fields", "foreign same public fields":
				third := NewJavaRef(id, nil, typ)
				third.VarUid = first.VarUid
				if variant == "foreign same public fields" {
					fourth := NewJavaRef(id, nil, typ)
					fourth.VarUid = first.VarUid
					MarkOriginalLocalWeb([]*JavaRef{third, fourth})
				}
				second = third
			case "changed UID":
				second.VarUid += "other"
			case "changed ID":
				second.Id = utils.NewRootVariableId()
			case "nil ID":
				second.Id = nil
			case "parameter after seal":
				second.IsParam = true
			case "receiver":
				second.IsThis = true
			case "custom":
				second.CustomValue = &CustomValue{}
			case "stack":
				second.StackVar = JavaNull
			case "different web":
				third := NewJavaRef(id, nil, typ)
				third.VarUid = first.VarUid
				MarkOriginalLocalWeb([]*JavaRef{second, third})
			case "clone":
				copy := *second
				second = &copy
			}
			want := variant == "original" || variant == "rename" || variant == "shared source ID change"
			if first.SameOriginalLocalWeb(second) != want || second.SameOriginalLocalWeb(first) != want {
				t.Fatal("borrowed an original definition web")
			}
		})
	}
}

func TestOriginalLocalWebRetainsTokenDuringTypePropagation(t *testing.T) {
	id := utils.NewRootVariableId()
	first, second := NewJavaRef(id, nil, types.NewJavaClass("Base")), NewJavaRef(id, nil, types.NewJavaClass("Base"))
	second.VarUid = first.VarUid
	MarkOriginalLocalWeb([]*JavaRef{first, second})
	token := first.originalLocalWeb
	if token == nil {
		t.Fatal("missing original cohort")
	}
	for i := 0; i < 20; i++ {
		MarkOriginalLocalWeb([]*JavaRef{second, first})
		if first.originalLocalWeb != token || second.originalLocalWeb != token {
			t.Fatal("type propagation replaced immutable cohort")
		}
	}
}
