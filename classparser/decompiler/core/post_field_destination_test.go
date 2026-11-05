package core

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestPostFieldTargetRequiresOriginalDestination(t *testing.T) {
	for _, kind := range []struct{ name, descriptor string }{{types.JavaInteger, "I"}, {types.JavaLong, "J"}, {types.JavaFloat, "F"}, {types.JavaDouble, "D"}} {
		for _, static := range []bool{false, true} {
			for _, change := range []string{"proved", "raw field", "forwarded receiver", "different field", "different owner", "different descriptor", "different receiver", "missing read witness", "missing stack witness", "changed stack seed", "wrong stack opcode", "missing origin", "changed origin", "ordinary local", "wrong write category", "nil field", "nil pool", "nil destination", "missing receiver", "field text only"} {
				name := kind.name + "/" + change
				if static {
					name += "/static"
				}
				t.Run(name, func(t *testing.T) {
					typ := types.NewJavaPrimer(kind.name)
					member := &values.JavaClassMember{Name: "Counter", Member: "word", Description: kind.descriptor, JavaType: typ}
					receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Counter"))
					var read, write values.JavaValue
					if static {
						field := *member
						field.OriginPC, field.HasOriginPC = 4, true
						if change != "missing read witness" {
							field.MarkOriginalFieldRead(member, 4)
						}
						read, write = &field, member
						if change == "missing origin" {
							field.HasOriginPC = false
						}
						if change == "changed origin" {
							field.OriginPC++
						}
						if change == "field text only" {
							field.Member = "different"
						}
					} else {
						field := values.NewRefMember(receiver, member.Member, typ)
						field.OriginPC, field.HasOriginPC = 4, true
						if change != "missing read witness" {
							field.MarkOriginalFieldRead(member, 4)
						}
						read, write = field, values.NewRefMember(receiver, member.Member, typ)
						if change == "missing origin" {
							field.HasOriginPC = false
						}
						if change == "changed origin" {
							field.OriginPC++
						}
						if change == "field text only" {
							field.Member = "different"
						}
					}
					code := OP_DUP_X1
					if static {
						code = OP_DUP
					}
					if kind.descriptor == "J" || kind.descriptor == "D" {
						if static {
							code = OP_DUP2
						} else {
							code = OP_DUP2_X1
						}
					}
					ref := values.NewJavaRef(utils.NewRootVariableId(), read, typ)
					if change == "wrong stack opcode" {
						code = OP_ILOAD_1
					}
					if change != "missing stack witness" && change != "ordinary local" {
						ref.MarkOriginalStackMaterialization(7, code, read)
					}
					var old values.JavaValue = ref
					switch change {
					case "raw field":
						old = read
					case "forwarded receiver":
						if !static {
							write.(*values.RefMember).Object = values.NewStackLifetimeUseView(receiver)
						}
					case "different field":
						member.Member = "other"
					case "different owner":
						member.Name = "Other"
					case "different descriptor":
						member.Description = "Z"
					case "different receiver":
						if !static {
							write.(*values.RefMember).Object = values.NewJavaRef(utils.NewRootVariableId(), nil, receiver.Type())
						} else {
							member.Name = "Other"
						}
					case "changed stack seed":
						ref.Val = values.NewJavaLiteral(1, typ)
					case "wrong write category":
						if static {
							write = values.NewRefMember(receiver, member.Member, typ)
						} else {
							write = member
						}
					case "nil field":
						old = (*values.RefMember)(nil)
					case "nil pool":
						member = nil
					case "nil destination":
						write = nil
					case "missing receiver":
						if !static {
							write.(*values.RefMember).Object = nil
						} else {
							write = nil
						}
					}
					got := originalPostFieldTarget(old, write, member)
					want := change == "proved" || change == "raw field" || change == "forwarded receiver"
					if (got != nil) != want || want && got != read {
						t.Fatalf("proved=%v want=%v", got != nil, want)
					}
				})
			}
		}
	}
}

func TestPostFieldTargetRejectsUnresolvedReceiverIdentity(t *testing.T) {
	for _, depth := range []int{0, 1, 31, 32, 33} {
		for _, same := range []bool{false, true} {
			t.Run(fmt.Sprintf("depth%d/same%v", depth, same), func(t *testing.T) {
				typ := types.NewJavaPrimer(types.JavaInteger)
				member := &values.JavaClassMember{Name: "Owner", Member: "value", Description: "I", JavaType: typ}
				var a values.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
				var b values.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
				if same {
					b = a
				}
				for i := 0; i < depth; i++ {
					a = values.NewStackLifetimeUseView(a)
					b = values.NewStackLifetimeUseView(b)
				}
				read := values.NewRefMember(a, member.Member, typ)
				read.OriginPC, read.HasOriginPC = 1, true
				read.MarkOriginalFieldRead(member, 1)
				write := values.NewRefMember(b, member.Member, typ)
				got := originalPostFieldTarget(read, write, member)
				if (got != nil) != (same && depth < 32) {
					t.Fatal("unresolved receivers cannot prove equality")
				}
			})
		}
	}
}

func TestPostFieldTargetRejectsForwardedMissingReceiver(t *testing.T) {
	for _, receiver := range []values.JavaValue{nil, (*values.JavaRef)(nil), (*values.SlotValue)(nil)} {
		typ := types.NewJavaPrimer(types.JavaInteger)
		member := &values.JavaClassMember{Name: "Owner", Member: "word", Description: "I", JavaType: typ}
		read := values.NewRefMember(values.NewStackLifetimeUseView(receiver), member.Member, typ)
		read.OriginPC, read.HasOriginPC = 1, true
		read.MarkOriginalFieldRead(member, 1)
		write := values.NewRefMember(values.NewStackLifetimeUseView(receiver), member.Member, typ)
		if originalPostFieldTarget(read, write, member) != nil {
			t.Fatal("absent receiver supplied identity proof")
		}
	}
}
