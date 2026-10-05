package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalFieldAssignmentKeepsImmutableStoreAndRenderingWitness(t *testing.T) {
	for _, change := range []string{"original", "renamed receiver", "replaced public renderer", "synthetic", "no origin", "different PC", "different field", "replaced target", "replaced value", "changed pool member", "nil renderer"} {
		t.Run(change, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
			field := NewRefMember(receiver, "field", typ)
			value := NewJavaLiteral(17, typ)
			member := NewJavaClassMember("Owner", "field", "I", typ)
			render := func(*class_context.ClassContext) string { return "original store" }
			a := NewOriginalFieldAssignmentExpression(field, value, member, 12, render)
			switch change {
			case "renamed receiver":
				receiver.Id = receiver.Id.Next()
			case "replaced public renderer":
				a.Render = func(*class_context.ClassContext) string { return "wrong store" }
			case "synthetic":
				a = NewAssignmentExpression(field, value, 12, render)
			case "no origin":
				a.HasOriginPC = false
			case "different PC":
				a.OriginPC = 13
			case "different field":
				field.Member = "different"
			case "replaced target":
				a.Target = NewRefMember(receiver, "field", typ)
			case "replaced value":
				a.Value = NewJavaLiteral(18, typ)
			case "changed pool member":
				member.Name, member.Member, member.Description = "Other", "different", "J"
			case "nil renderer":
				a = NewOriginalFieldAssignmentExpression(field, value, member, 12, nil)
			}
			pc, owner, name, desc, known := a.OriginalFieldStoreWitness()
			want := change == "original" || change == "renamed receiver" || change == "replaced public renderer" || change == "changed pool member"
			if known != want {
				t.Fatalf("known=%v want=%v", known, want)
			}
			if known && (pc != 12 || owner != "Owner" || name != "field" || desc != "I") {
				t.Fatal("original pool identity changed")
			}
			if change == "replaced public renderer" && a.String(&class_context.ClassContext{}) != "original store" {
				t.Fatal("public callback replaced witnessed original rendering")
			}
		})
	}
}
func TestOriginalStackMaterializationWitnessSurvivesOnlyLogicalReferenceClones(t *testing.T) {
	for _, change := range []string{"original", "shallow clone", "renamed ID", "remark immutable", "synthetic", "this", "parameter", "custom", "stack alias", "changed original value", "different RHS"} {
		t.Run(change, func(t *testing.T) {
			value := NewJavaLiteral(17, types.NewJavaPrimer(types.JavaInteger))
			r := NewJavaRef(utils.NewRootVariableId().Next(), value, value.Type())
			r.MarkOriginalStackMaterialization(12, 90, value)
			rhs := JavaValue(value)
			switch change {
			case "shallow clone":
				copy := *r
				r = &copy
			case "renamed ID":
				r.Id = r.Id.Next()
			case "remark immutable":
				r.MarkOriginalStackMaterialization(13, 89, NewJavaLiteral(18, value.Type()))
			case "synthetic":
				r = NewJavaRef(r.Id, value, value.Type())
			case "this":
				r.IsThis = true
			case "parameter":
				r.IsParam = true
			case "custom":
				r.CustomValue = &CustomValue{}
			case "stack alias":
				r.StackVar = value
			case "changed original value":
				r.Val = NewJavaLiteral(18, value.Type())
			case "different RHS":
				rhs = NewJavaLiteral(17, value.Type())
			}
			pc, kind, known := r.OriginalStackMaterializationWitness(rhs)
			want := change == "original" || change == "shallow clone" || change == "renamed ID" || change == "remark immutable"
			if known != want || known && (pc != 12 || kind != 90) {
				t.Fatalf("witness=%d/%d/%v want=%v", pc, kind, known, want)
			}
		})
	}
}
