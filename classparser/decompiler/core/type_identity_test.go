package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNestedNameDoesNotProveSubtype(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "audit.Outer"}
	member := values.NewJavaClassMember("audit.Outer", "value", "I", types.NewJavaPrimer(types.JavaInteger))
	recv := values.NewJavaLiteral(nil, types.NewJavaClass("audit.Outer$Nested"))
	if got := castAnonSubclassReceiverForOwnField(recv, member, ctx); got != recv {
		t.Fatal("unresolved nesting inferred inheritance")
	}
	ctx.SiblingSuperTypes = func(string) ([]string, bool) { return []string{"java/lang/Object"}, true }
	if got := castAnonSubclassReceiverForOwnField(recv, member, ctx); got != recv {
		t.Fatal("unrelated nesting inferred inheritance")
	}
	ctx.SiblingSuperTypes = func(n string) ([]string, bool) {
		if n == "audit/Outer$Nested" {
			return []string{"audit/Outer"}, true
		}
		return nil, false
	}
	if got := castAnonSubclassReceiverForOwnField(recv, member, ctx); got == recv {
		t.Fatal("proven private-field receiver subtype not cast")
	}
	if types.IsReferenceSubtypeBridged("other.List", "java.util.List", nil) {
		t.Fatal("suffix inferred type identity")
	}
	if !types.IsSubtypeVia("a.Outer$Nested", "a.Outer$Nested", nil) {
		t.Fatal("exact identity needs no resolver")
	}
}
