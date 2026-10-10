package class_context

import (
	"testing"

	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
)

func TestStaticFieldSelectionClosesBothValueBindings(t *testing.T) {
	for _, row := range []struct {
		name, owner, member, want string
		ctx                       ClassContext
	}{
		{"own field", "Root", "value", "value", ClassContext{ClassName: "Root", SourceValueNameShadow: func(s string) bool { return s == "value" }}},
		{"own parameter", "Root", "value", "Root.value", ClassContext{ClassName: "Root", Arguments: []string{"value"}}},
		{"own local", "Root", "value", "Root.value", ClassContext{ClassName: "Root", LocalNames: map[*coreutils.VariableId]string{nil: "value"}}},
		{"both local names", "Root", "value", "((Root)null).value", ClassContext{ClassName: "Root", Arguments: []string{"value", "Root"}}},
		{"default package field", "Root", "value", "((Root)null).value", ClassContext{SourceValueNameShadow: func(s string) bool { return s == "Root" }}},
		{"qualified fallback", "scope.Root", "value", "scope.Root.value", ClassContext{Arguments: []string{"Root"}}},
		{"qualified root obscured", "scope.Root", "value", "((Root)null).value", ClassContext{Arguments: []string{"Root", "scope"}}},
		{"forced own selection", "Root", "value", "Root.value", ClassContext{ClassName: "Root", QualifiedStaticFields: true}},
		{"interface field primary", "Constants", "value", "((Constants)null).value", ClassContext{Arguments: []string{"Constants"}}},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := row.ctx.StaticFieldSelection(row.owner, row.member); got != row.want {
				t.Fatalf("got=%q want=%q", got, row.want)
			}
		})
	}
}

func TestStaticFieldSelectionRefusesAnObscuredTypeNamespace(t *testing.T) {
	for _, ctx := range []ClassContext{
		{TypeParams: []string{"Root"}, Arguments: []string{"Root"}},
		{TypeParams: []string{"Root"}, LexicalTypeNames: map[string]bool{"scope": true}, Arguments: []string{"Root", "scope"}},
	} {
		owner := "Root"
		if ctx.LexicalTypeNames != nil {
			owner = "scope.Root"
		}
		ctx.StaticFieldSelection(owner, "value")
		if ctx.StaticMethodImports == nil || ctx.StaticMethodImports.Error() == nil {
			t.Fatal("unproved type binding did not fail the source certificate")
		}
	}
}
