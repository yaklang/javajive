package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/fieldbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestObjectReceiverFieldCastOnReadAndWrite(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	objType := types.NewJavaClass("java.lang.Object")

	for _, variant := range []string{
		"read_partial_metadata",
		"write_partial_metadata",
		"read_closed_world_casts_object_to_bytecode_owner",
		"skip_this",
		"skip_param",
		"skip_concrete_local",
		"skip_object_owner",
		"skip_array_owner",
		"skip_missing_witness",
		"skip_changed_pc",
		"skip_kill_switch",
	} {
		t.Run(variant, func(t *testing.T) {
			id := utils.NewRootVariableId()
			recv := NewJavaRef(id, nil, objType)
			ownerName := "p.Modifier$Node"
			field := NewRefMember(recv, "kindSet", intType)
			field.HasOriginPC = true
			field.OriginPC = 17
			ctx := &class_context.ClassContext{
				ClassName:   "p.Caller",
				PackageName: "p",
				LocalNames:  map[*utils.VariableId]string{id: "var8"},
			}
			var want string
			switch variant {
			case "read_partial_metadata":
				field.MarkOriginalFieldRead(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				want = "((Modifier$Node)(var8)).kindSet"
			case "write_partial_metadata":
				field.MarkOriginalFieldWrite(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				want = "((Modifier$Node)(var8)).kindSet"
			case "read_closed_world_casts_object_to_bytecode_owner":
				field.MarkOriginalFieldRead(NewJavaClassMember("p.SubNode", "kindSet", "I", intType), 17)
				classes := map[string]fieldbinding.Class{
					"p/BaseNode": {Name: "p/BaseNode", Accessible: true, Complete: true, Fields: []fieldbinding.Field{{Name: "kindSet", Descriptor: "I", Access: 1}}},
					"p/SubNode":  {Name: "p/SubNode", Super: "p/BaseNode", Accessible: true, Complete: true},
				}
				ctx.OriginalFieldDeclarations = func(n string) (fieldbinding.Class, bool) { c, ok := classes[n]; return c, ok }
				ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
					c, ok := classes[n]
					var parents []string
					if c.Super != "" {
						parents = append(parents, c.Super)
					}
					return callbinding.Class{Name: c.Name, Parents: parents, ParentsComplete: c.Complete}, ok
				}
				want = "((SubNode)(var8)).kindSet"
			case "skip_this":
				recv.IsThis = true
				field.MarkOriginalFieldRead(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				want = "this.kindSet"
			case "skip_param":
				recv.IsParam = true
				field.MarkOriginalFieldRead(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				want = "var8.kindSet"
			case "skip_concrete_local":
				recv = NewJavaRef(id, nil, types.NewJavaClass(ownerName))
				field = NewRefMember(recv, "kindSet", intType)
				field.HasOriginPC = true
				field.OriginPC = 17
				field.MarkOriginalFieldRead(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				want = "var8.kindSet"
			case "skip_object_owner":
				field.MarkOriginalFieldRead(NewJavaClassMember("java.lang.Object", "kindSet", "I", intType), 17)
				want = "var8.kindSet"
			case "skip_array_owner":
				field.MarkOriginalFieldRead(NewJavaClassMember("[Ljava/lang/Object;", "kindSet", "I", intType), 17)
				want = "var8.kindSet"
			case "skip_missing_witness":
				want = "var8.kindSet"
			case "skip_changed_pc":
				field.MarkOriginalFieldRead(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				field.OriginPC = 18
				want = "var8.kindSet"
			case "skip_kill_switch":
				field.MarkOriginalFieldRead(NewJavaClassMember(ownerName, "kindSet", "I", intType), 17)
				t.Setenv("JDEC_OBJECT_RECV_FIELD_CAST_OFF", "1")
				want = "var8.kindSet"
			}
			got := field.String(ctx)
			if got != want {
				t.Fatalf("RefMember.String() = %q, want %q", got, want)
			}
		})
	}
}
