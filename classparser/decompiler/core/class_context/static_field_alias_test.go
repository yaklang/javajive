package class_context

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"testing"
)

func TestStaticStorageAlphaBindingKeepsPhysicalOwnerAndValueScope(t *testing.T) {
	for _, tt := range []struct {
		name, owner, member, want string
		shadow                    bool
	}{
		{"owned", "p.Owner", "$assertionsDisabled", "$storage", false},
		{"foreign owner", "p.Other", "$assertionsDisabled", "Other.$assertionsDisabled", false},
		{"different field", "p.Owner", "ordinary", "ordinary", false},
		{"local shadows alias", "p.Owner", "$assertionsDisabled", "Owner.$storage", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			context := &ClassContext{ClassName: "p.Owner", PackageName: "p", StaticFieldAliases: map[string]string{"p/Owner\x00$assertionsDisabled": "$storage"}}
			if tt.shadow {
				context.LocalNames = map[*utils.VariableId]string{utils.NewRootVariableId(): "$storage"}
			}
			if got := context.StaticFieldSelection(tt.owner, tt.member); got != tt.want {
				t.Fatalf("source field=%q want=%q", got, tt.want)
			}
			if context.StaticFieldAliases["p/Owner\x00$assertionsDisabled"] != "$storage" {
				t.Fatal("writer mutated the binding")
			}
		})
	}
}
