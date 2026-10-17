package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"
)

func TestSignatureVariableCannotBorrowSameSpelledClassSourceIdentity(t *testing.T) {
	for _, row := range []struct{ signature, want string }{{"TName;", "Name"}, {"[TName;", "Name[]"}, {"Ljava/util/List<TName;>;", "List<Name>"}} {
		t.Run(row.signature, func(t *testing.T) {
			calls := 0
			ctx := &class_context.ClassContext{TypeParams: []string{"Name"}}
			ctx.DeclarationSourceName = func(name string) (string, bool) {
				if name == "Name" {
					calls++
					return "ActualClass", true
				}
				return "", false
			}
			value := ParseSignature(row.signature)
			if value == nil {
				t.Fatal("signature")
			}
			if got := value.String(ctx); got != row.want {
				t.Fatalf("formal source=%q want=%q", got, row.want)
			}
			if got := value.Copy().String(ctx); got != row.want {
				t.Fatalf("copied formal source=%q", got)
			}
			if calls != 0 {
				t.Fatalf("formal borrowed class resolver %d times", calls)
			}
			class := ParseSignature("LName;")
			if class == nil || class.String(ctx) != "ActualClass" || calls != 1 {
				t.Fatal("actual class must retain declaration lookup")
			}
		})
	}
}
