package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestBooleanFieldWordRequiresImmutableOriginalDeclarationAndReadPC(t *testing.T) {
	for _, static := range []bool{false, true} {
		for _, change := range []string{"original", "shallow clone", "pool changed", "remark immutable", "synthetic", "no origin", "other PC", "different name", "original int descriptor", "retyped int", "unknown local", "shared dup"} {
			t.Run(map[bool]string{false: "instance", true: "static"}[static]+"/"+change, func(t *testing.T) {
				boolean := types.NewJavaPrimer(types.JavaBoolean)
				integer := types.NewJavaPrimer(types.JavaInteger)
				member := NewJavaClassMember("Owner", "flag", "Z", boolean)
				if change == "original int descriptor" {
					member.Description = "I"
				}
				receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
				var field JavaValue
				if static {
					copy := *member
					copy.OriginPC, copy.HasOriginPC = 12, true
					if change != "synthetic" {
						copy.MarkOriginalFieldRead(member, 12)
					}
					field = &copy
				} else {
					f := NewRefMember(receiver, "flag", boolean)
					f.OriginPC, f.HasOriginPC = 12, true
					if change != "synthetic" {
						f.MarkOriginalFieldRead(member, 12)
					}
					field = f
				}
				switch change {
				case "shallow clone":
					switch f := field.(type) {
					case *RefMember:
						c := *f
						field = &c
					case *JavaClassMember:
						c := *f
						field = &c
					}
				case "pool changed":
					member.Name, member.Member, member.Description = "Other", "other", "I"
				case "remark immutable":
					other := NewJavaClassMember("Other", "other", "I", integer)
					switch f := field.(type) {
					case *RefMember:
						f.MarkOriginalFieldRead(other, 13)
					case *JavaClassMember:
						f.MarkOriginalFieldRead(other, 13)
					}
				case "no origin":
					switch f := field.(type) {
					case *RefMember:
						f.HasOriginPC = false
					case *JavaClassMember:
						f.HasOriginPC = false
					}
				case "other PC":
					switch f := field.(type) {
					case *RefMember:
						f.OriginPC++
					case *JavaClassMember:
						f.OriginPC++
					}
				case "different name":
					switch f := field.(type) {
					case *RefMember:
						f.Member = "other"
					case *JavaClassMember:
						f.Member = "other"
					}
				case "retyped int":
					switch f := field.(type) {
					case *RefMember:
						f.JavaType = integer
					case *JavaClassMember:
						f.JavaType = integer
					}
				case "unknown local":
					field = NewJavaRef(utils.NewRootVariableId(), field, boolean)
				case "shared dup":
					r := NewJavaRef(utils.NewRootVariableId(), field, boolean)
					r.MarkOriginalStackMaterialization(15, 89, field)
					field = r
				}
				want := change == "original" || change == "shallow clone" || change == "pool changed" || change == "remark immutable" || change == "shared dup"
				if got := OriginalBooleanStackWord(field); got != want {
					t.Fatalf("canonical0/1=%v want%v", got, want)
				}
				if got := CoerceIntAssignRHS(integer, field, &class_context.ClassContext{}); (got != field) != want {
					t.Fatal("consumer did not require original read")
				}
			})
		}
	}
}
func TestBooleanFieldWordSharedProofIsBoundedAndRejectsCycles(t *testing.T) {
	b := types.NewJavaPrimer(types.JavaBoolean)
	base := JavaValue(NewJavaLiteral(true, b))
	for i := 0; i < 65; i++ {
		r := NewJavaRef(utils.NewRootVariableId(), base, b)
		r.MarkOriginalStackMaterialization(i, 89, base)
		base = r
	}
	if OriginalBooleanStackWord(base) {
		t.Fatal("depth budget ignored")
	}
	r := NewJavaRef(utils.NewRootVariableId(), nil, b)
	r.Val = r
	r.MarkOriginalStackMaterialization(1, 89, r)
	if OriginalBooleanStackWord(r) {
		t.Fatal("cycle accepted")
	}
	for _, word := range []int{2, 3, -1, -2} {
		if OriginalBooleanStackWord(NewJavaLiteral(word, b)) {
			t.Fatal("noncanonical word admitted")
		}
	}
}
