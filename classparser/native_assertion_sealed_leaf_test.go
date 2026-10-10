package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeAssertionReadIdentityRequiresSealedOperandFreeLeaf(t *testing.T) {
	for _, variant := range []string{"sealed break", "sealed continue", "anchor", "opaque break", "incidental info", "hidden throw", "invalid label", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			flag := &values.JavaClassMember{Name: "p.Owner", Member: nativeAssertionField, Description: "Z", JavaType: types.NewJavaPrimer(types.JavaBoolean), OriginPC: 7, HasOriginPC: true}
			var leaf statements.Statement = statements.NewSourceTransferStatement("break", "")
			switch variant {
			case "sealed continue":
				leaf = statements.NewSourceTransferStatement("continue", "LOOP")
			case "anchor":
				leaf = statements.NewSourceAnchorStatement()
			case "opaque break":
				leaf = statements.NewCustomStatement(func(*class_context.ClassContext) string { return "break" }, nil)
			case "incidental info":
				x := statements.NewSourceTransferStatement("break", "")
				x.Info = flag
				leaf = x
			case "invalid label":
				x := statements.NewSourceTransferStatement("break", "")
				x.LoopTargetLabel = "LOOP;read()"
				leaf = x
			case "hidden throw":
				x := statements.NewSourceTransferStatement("break", "")
				x.ThrownValue = flag
				leaf = x
			case "cycle":
				x := &statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				x.Body = []statements.Statement{x}
				leaf = x
			}
			body := []statements.Statement{&statements.ConditionStatement{Condition: flag}, leaf}
			got := nativeAssertionSourceReads(body, "p/Owner", map[int]*nativeAssertionPacket{7: {}}, nil)
			want := variant == "sealed break" || variant == "sealed continue" || variant == "anchor" || variant == "incidental info"
			if got != want {
				t.Fatalf("read closure=%v want=%v", got, want)
			}
			if want && variant != "incidental info" {
				if _, _, control := catchSourceChildren(leaf); control {
					t.Fatal("read identity gave effect/control permission")
				}
			}
		})
	}
}
