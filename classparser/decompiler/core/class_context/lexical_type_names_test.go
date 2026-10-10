package class_context

import "testing"

func TestLexicalMemberNamesQualifyShadowedExternalTypes(t *testing.T) {
	ctx := &ClassContext{PackageName: "scope.member", LexicalTypeNames: map[string]bool{"Iterator": true, "String": true, "Sibling": true, "Map": true}}
	for _, row := range []struct{ binary, source string }{{"java.util.Iterator", "java.util.Iterator"}, {"java.lang.String", "java.lang.String"}, {"scope.member.Sibling", "scope.member.Sibling"}, {"java.util.Map$Entry", "java.util.Map.Entry"}} {
		if got := ctx.ShortTypeName(row.binary); got != row.source {
			t.Fatalf("%s -> %s", row.binary, got)
		}
	}
	clone := ctx.CloneForRetry()
	clone.LexicalTypeNames["String"] = false
	if !ctx.LexicalTypeNames["String"] {
		t.Fatal("retry changed lexical scope")
	}
	if got := ctx.ShortTypeName("java.lang.Integer"); got != "Integer" {
		t.Fatal(got)
	}
}

func TestEnclosingFormalDeclarationsCloneForRetry(t *testing.T) {
	ctx := &ClassContext{LexicalTypeParamSignatures: []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}}
	clone := ctx.CloneForRetry()
	clone.LexicalTypeParamSignatures[0] = "<T:Ljava/lang/String;>Ljava/lang/Object;"
	if ctx.LexicalTypeParamSignatures[0] != "<T:Ljava/lang/Number;>Ljava/lang/Object;" {
		t.Fatal("retry changed the original enclosing declaration")
	}
}
