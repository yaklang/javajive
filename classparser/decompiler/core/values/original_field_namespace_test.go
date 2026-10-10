package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalFieldReadOwnerNamespaceKeepsExactSymbol(t *testing.T) {
	for _, symbol := range []string{"Owner", "packet/Owner", "independent/scope/Owner$Nested"} {
		for _, seed := range []string{symbol, strings.ReplaceAll(symbol, "/", ".")} {
			for _, query := range []string{symbol, strings.ReplaceAll(symbol, "/", ".")} {
				for _, static := range []bool{false, true} {
					t.Run(seed+"->"+query+"/"+map[bool]string{false: "instance", true: "static"}[static], func(t *testing.T) {
						typ := types.NewJavaClass("Mode")
						member := NewJavaClassMember(seed, "mode", "LMode;", typ)
						if static {
							field := *member
							field.OriginPC, field.HasOriginPC = 7, true
							field.MarkOriginalFieldRead(member, 7)
							if !field.OriginalStaticFieldRead(7, query, "mode", "LMode;") {
								t.Fatal("same original static owner refused across namespaces")
							}
						} else {
							field := NewRefMember(JavaNull, "mode", typ)
							field.OriginPC, field.HasOriginPC = 7, true
							field.MarkOriginalFieldRead(member, 7)
							if !field.OriginalInstanceFieldRead(7, query, "mode", "LMode;") {
								t.Fatal("same original instance owner refused across namespaces")
							}
						}
					})
				}
			}
		}
	}
	for _, variant := range []string{"other package", "other nested owner", "nested separator", "descriptor", "member", "pc", "no origin", "no witness", "restamp", "static owner renamed", "instance field renamed", "static method handle"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("Mode")
			member := NewJavaClassMember("packet.Owner$Nested", "mode", "LMode;", typ)
			static := *member
			instance := NewRefMember(JavaNull, "mode", typ)
			static.OriginPC, static.HasOriginPC = 7, true
			instance.OriginPC, instance.HasOriginPC = 7, true
			if variant != "no witness" {
				static.MarkOriginalFieldRead(member, 7)
				instance.MarkOriginalFieldRead(member, 7)
			}
			pc, owner, name, desc := 7, "packet/Owner$Nested", "mode", "LMode;"
			wantStatic, wantInstance := false, false
			switch variant {
			case "other package":
				owner = "decoy/Owner$Nested"
			case "other nested owner":
				owner = "packet/Other$Nested"
			case "nested separator":
				owner = "packet.Owner.Nested"
			case "descriptor":
				desc = "Lpacket.Mode;"
			case "member":
				name = "other"
			case "pc":
				pc++
			case "no origin":
				static.HasOriginPC, instance.HasOriginPC = false, false
			case "restamp":
				other := NewJavaClassMember("decoy.Owner$Nested", "mode", "LMode;", typ)
				static.MarkOriginalFieldRead(other, 9)
				instance.MarkOriginalFieldRead(other, 9)
				owner, pc = "decoy/Owner$Nested", 9
			case "static owner renamed":
				static.Name = "decoy.Owner$Nested"
				wantInstance = true
			case "instance field renamed":
				instance.Member = "other"
				wantStatic = true
			case "static method handle":
				static.RefKind = 2
				wantInstance = true
			}
			if static.OriginalStaticFieldRead(pc, owner, name, desc) != wantStatic || instance.OriginalInstanceFieldRead(pc, owner, name, desc) != wantInstance {
				t.Fatal("owner normalization changed original field certificate boundaries")
			}
		})
	}
}
