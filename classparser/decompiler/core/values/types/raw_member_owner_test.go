package types

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestHiddenLexicalMemberRenderingErasesWholeOwnerChain(t *testing.T) {
	for _, row := range []struct{ name, signature, want string }{
		{"hidden owner", "Lproof/Outer<TT;>.Inner;", "proof.Outer.Inner"},
		{"hidden owner with own member formal", "Lproof/Outer<TT;>.Inner<TU;>;", "proof.Outer.Inner"},
		{"hidden member with visible owner", "Lproof/Outer<TU;>.Inner<TT;>;", "proof.Outer.Inner"},
		{"hidden middle", "Lproof/Outer<TU;>.Middle<TT;>.Inner<TV;>;", "proof.Outer.Middle.Inner"},
		{"direct wildcard", "Lproof/Outer<+TT;>.Inner;", "proof.Outer.Inner"},
		{"visible owners", "Lproof/Outer<TU;>.Inner<TV;>;", "proof.Outer<U>.Inner<V>"},
		{"concrete owner", "Lproof/Outer<Ljava/lang/String;>.Inner<TU;>;", "proof.Outer<String>.Inner<U>"},
		{"array bound stays argument", "Lproof/Outer<[TT;>.Inner;", "proof.Outer<Number[]>.Inner"},
	} {
		t.Run(row.name, func(t *testing.T) {
			value, original := ParseSignature(row.signature), ParseSignature(row.signature)
			ctx := &class_context.ClassContext{TypeParams: []string{"U", "V"}, RawEraseTypeVars: map[string]bool{"T": true}, StandaloneEraseTypeVars: map[string]string{"T": "java.lang.Number"}, DeclarationSourceName: func(name string) (string, bool) {
				if strings.HasPrefix(name, "proof.Outer") {
					return strings.ReplaceAll(name, "$", "."), true
				}
				return "", false
			}}
			if got := value.String(ctx); got != row.want {
				t.Fatalf("source owner binding=%q want=%q", got, row.want)
			}
			if !reflect.DeepEqual(value, original) {
				t.Fatal("rendering changed original signature graph")
			}
		})
	}
}
