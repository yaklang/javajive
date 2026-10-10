package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalInstanceFieldStoreKeepsReceiverRHSAndPhysicalWrite(t *testing.T) {
	for _, change := range []string{"original", "canonical owner", "invalid negative PC", "invalid oversized PC", "forwarding slots", "statement clone", "receiver rename", "repeated mark", "missing witness", "wrong PC", "no PC", "different target", "wrong member", "same-type other receiver", "different receiver slot", "different RHS", "different RHS slot", "array target", "declaration", "nil RHS", "nil receiver", "deep forwarding"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("sample.Cell"))
			rhs := values.NewJavaLiteral(7, typ)
			target := values.NewRefMember(receiver, "length", typ)
			left, right := values.NewSlotValue(receiver, receiver.Type()), values.NewSlotValue(rhs, rhs.Type())
			if change == "forwarding slots" {
				target.Object = left
			}
			assign := NewAssignStatement(target, right, false)
			assign.OriginPC = 17
			assign.HasOriginPC = true
			member := &values.JavaClassMember{Name: "sample/Cell", Member: "length", Description: "I"}
			if change == "canonical owner" {
				member.Name = "sample.Cell"
			}
			if change == "invalid negative PC" {
				assign.OriginPC = -1
			}
			if change == "invalid oversized PC" {
				assign.OriginPC = 65536
			}
			if change != "missing witness" {
				assign.MarkOriginalInstanceFieldStore(member, 17)
			}
			want := change == "original" || change == "canonical owner" || change == "forwarding slots" || change == "statement clone" || change == "receiver rename" || change == "repeated mark"
			switch change {
			case "statement clone":
				copy := *assign
				assign = &copy
			case "receiver rename":
				receiver.Id.SetName("different")
			case "repeated mark":
				assign.MarkOriginalInstanceFieldStore(&values.JavaClassMember{Name: "different", Member: "other", Description: "J"}, 19)
			case "wrong PC":
				assign.OriginPC++
			case "no PC":
				assign.HasOriginPC = false
			case "different target":
				assign.LeftValue = values.NewRefMember(receiver, "length", typ)
			case "wrong member":
				target.Member = "other"
			case "same-type other receiver":
				target.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, receiver.Type())
			case "different receiver slot":
				target.Object = left
				left.ResetValue(values.NewJavaRef(utils.NewRootVariableId(), nil, receiver.Type()))
			case "different RHS":
				assign.JavaValue = values.NewJavaLiteral(7, typ)
			case "different RHS slot":
				right.ResetValue(values.NewJavaLiteral(7, typ))
			case "array target":
				assign.ArrayMember = &values.JavaArrayMember{}
			case "declaration":
				assign.IsDeclare = true
			case "nil RHS":
				assign.JavaValue = nil
			case "nil receiver":
				target.Object = nil
			case "deep forwarding":
				for i := 0; i < 33; i++ {
					assign.JavaValue = values.NewSlotValue(assign.JavaValue, typ)
				}
			}
			pc, owner, name, desc, known := assign.OriginalInstanceFieldStore()
			if known != want || known && (pc != 17 || owner != "sample/Cell" || name != "length" || desc != "I") {
				t.Fatalf("store=%d/%s/%s/%s/%v want=%v", pc, owner, name, desc, known, want)
			}
		})
	}
}
