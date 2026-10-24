package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/fieldbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalFieldSelectionDoesNotTurnWriteIntoBooleanRead(t *testing.T) {
	for _, variant := range []string{"write", "read", "write cannot replace read", "changed pool", "synthetic", "missing origin", "changed PC", "changed name", "missing declarations", "missing hierarchy", "incomplete declarations", "different descriptor", "unrelated receiver", "negative PC", "oversized PC"} {
		t.Run(variant, func(t *testing.T) {
			boolean := types.NewJavaPrimer(types.JavaBoolean)
			receiver := NewJavaRef(nil, nil, types.NewJavaClass("p.Child"))
			field := NewRefMember(receiver, "word", boolean)
			member := NewJavaClassMember("p.Base", "word", "Z", boolean)
			classes := map[string]fieldbinding.Class{
				"p/Base":  {Name: "p/Base", Accessible: true, Complete: true, Fields: []fieldbinding.Field{{Name: "word", Descriptor: "Z", Access: 1}}},
				"p/Child": {Name: "p/Child", Super: "p/Base", Accessible: true, Complete: true, Fields: []fieldbinding.Field{{Name: "word", Descriptor: "Z", Access: 1}}},
				"p/Other": {Name: "p/Other", Accessible: true, Complete: true, Fields: []fieldbinding.Field{{Name: "word", Descriptor: "Z", Access: 1}}},
			}
			ctx := &class_context.ClassContext{ClassName: "p.Access"}
			ctx.OriginalFieldDeclarations = func(n string) (fieldbinding.Class, bool) { c, ok := classes[n]; return c, ok }
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
				c, ok := classes[n]
				var parents []string
				if c.Super != "" {
					parents = append(parents, c.Super)
				}
				return callbinding.Class{Name: c.Name, Parents: parents, ParentsComplete: c.Complete}, ok
			}
			if variant == "read" || variant == "write cannot replace read" {
				field.OriginPC, field.HasOriginPC = 12, true
				field.MarkOriginalFieldRead(member, 12)
			} else if variant != "synthetic" {
				field.MarkOriginalFieldWrite(member, 12)
			}
			want := variant == "write" || variant == "read" || variant == "write cannot replace read" || variant == "changed pool"
			switch variant {
			case "write cannot replace read":
				field.MarkOriginalFieldWrite(NewJavaClassMember("p.Other", "word", "I", boolean), 42)
			case "changed pool":
				member.Name, member.Member, member.Description = "p.Other", "other", "I"
			case "missing origin":
				field.HasOriginPC = false
			case "changed PC":
				field.OriginPC++
			case "changed name":
				field.Member = "other"
			case "missing declarations":
				ctx.OriginalFieldDeclarations = nil
			case "missing hierarchy":
				ctx.InvocationMetadata = nil
			case "incomplete declarations":
				c := classes["p/Child"]
				c.Complete = false
				classes[c.Name] = c
			case "different descriptor":
				c := classes["p/Base"]
				c.Fields[0].Descriptor = "I"
				classes[c.Name] = c
			case "unrelated receiver":
				field.Object = NewJavaRef(nil, nil, types.NewJavaClass("p.Other"))
			case "negative PC":
				field.OriginPC = -1
			case "oversized PC":
				field.OriginPC = 65536
			}
			selected := field.sourceFieldReceiver(ctx)
			if (selected != nil) != want {
				t.Fatalf("binding=%v want=%v", selected != nil, want)
			}
			if selected != nil {
				cast, ok := selected.(*CastExpression)
				if !ok || cast.Value != receiver {
					t.Fatal("changed evaluated receiver or selected a different field owner")
				}
				if name, known := types.RawClassFQN(cast.TargetType); !known || name != "p.Base" {
					t.Fatal("selected a different physical field owner")
				}
			}
			read := variant == "read" || variant == "write cannot replace read"
			if got := OriginalBooleanStackWord(field); got != read {
				t.Fatalf("boolean read=%v want=%v; a field store cannot normalize a loaded word", got, read)
			}
		})
	}
}
