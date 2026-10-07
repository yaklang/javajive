package class_context

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"testing"
)

func TestStaticClassOwnerSeparatesTypeAndValueNamespaces(t *testing.T) {
	for _, row := range []struct {
		name        string
		ctx         *ClassContext
		owner, want string
	}{
		{"ordinary", &ClassContext{}, "java.lang.Float", "Float"},
		{"field", &ClassContext{SourceValueNameShadow: func(s string) bool { return s == "Float" }}, "java.lang.Float", "java.lang.Float"},
		{"parameter", &ClassContext{Arguments: []string{"Float"}}, "java.lang.Float", "java.lang.Float"},
		{"local", &ClassContext{LocalNames: map[*coreutils.VariableId]string{nil: "Float"}}, "java.lang.Float", "java.lang.Float"},
		{"qualified first value", &ClassContext{TypeParams: []string{"Float"}, Arguments: []string{"java"}}, "java.lang.Float", "((java.lang.Float)null)"},
		{"qualified harmless leaf", &ClassContext{TypeParams: []string{"Float"}, Arguments: []string{"Float"}}, "java.lang.Float", "java.lang.Float"},
		{"class shadow", &ClassContext{LexicalTypeNames: map[string]bool{"Float": true}}, "java.lang.Float", "java.lang.Float"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := row.ctx.StaticClassOwner(row.owner); got != row.want {
				t.Fatalf("got=%q want=%q", got, row.want)
			}
		})
	}
}

func TestNewPlatformOwnerChecksUnreferencedSamePackageDeclaration(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		ctx := &ClassContext{PackageName: "sample", InvocationMetadata: func(name string) (callbinding.Class, bool) {
			if name != "sample/Float" {
				t.Fatalf("queried wrong identity %q", name)
			}
			identity := name
			if wrong {
				identity = "other/Float"
			}
			return callbinding.Class{Name: identity}, true
		}}
		want := "java.lang.Float"
		if wrong {
			want = "Float"
		}
		if got := ctx.ShortTypeName("java.lang.Float"); got != want {
			t.Fatalf("wrong=%v name=%q want=%q", wrong, got, want)
		}
	}
}
