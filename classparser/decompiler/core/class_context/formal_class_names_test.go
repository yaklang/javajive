package class_context

import "testing"

func TestShortTypeNamePreservesClassAndFormalDeclarationIdentity(t *testing.T) {
	for _, row := range []struct {
		name, input string
		formals     []string
		want        string
	}{
		{"java lang class", "java.lang.String", []string{"String"}, "java.lang.String"},
		{"bare formal", "String", []string{"String"}, "String"},
		{"unrelated formal", "java.lang.String", []string{"T"}, "String"},
		{"same package class", "sample.Node", []string{"Node"}, "sample.Node"},
		{"external class", "java.util.ArrayList", []string{"ArrayList"}, "java.util.ArrayList"},
		{"external nested owner", "java.util.Map$Entry", []string{"Map"}, "java.util.Map.Entry"},
		{"nested leaf formal is not owner", "java.util.Map$Entry", []string{"Entry"}, "Map.Entry"},
		{"bare dollar formal", "T$Scope", []string{"T$Scope"}, "T$Scope"},
	} {
		t.Run(row.name, func(t *testing.T) {
			c := &ClassContext{PackageName: "sample", TypeParams: row.formals}
			if got := c.ShortTypeName(row.input); got != row.want {
				t.Fatalf("name=%q want=%q", got, row.want)
			}
		})
	}
}
func TestMethodClassNameShadowDoesNotLeakAcrossRenderContexts(t *testing.T) {
	c := &ClassContext{TypeParams: []string{"T"}}
	copy := c.CloneForRetry()
	copy.TypeParams = append(copy.TypeParams, "String")
	if copy.ShortTypeName("java.lang.String") != "java.lang.String" || c.ShortTypeName("java.lang.String") != "String" {
		t.Fatal("method declaration binding leaked to sibling")
	}
}
