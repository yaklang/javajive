package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestSharedSwitchTailRequiresEveryNormalPath(t *testing.T) {
	br := statements.NewSourceTransferStatement("break", "")
	ret := statements.NewReturnStatement(nil)
	nested := statements.NewSwitchStatement(values.JavaNull, []*statements.CaseItem{{IntValue: 1, Body: []statements.Statement{br}}, {IsDefault: true, Body: []statements.Statement{br}}})
	terminal := statements.NewSwitchStatement(values.JavaNull, []*statements.CaseItem{{IntValue: 1, Body: []statements.Statement{ret}}, {IsDefault: true, Body: []statements.Statement{ret}}})
	effect := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "touch()" }, nil)
	branch := func(a, b []statements.Statement) statements.Statement {
		return statements.NewIfStatement(values.JavaNull, a, b)
	}
	for _, tc := range []struct {
		name string
		tail statements.Statement
		want bool
	}{
		{"direct shared exit", nested, true},
		{"outer abrupt inner normal", branch([]statements.Statement{br}, []statements.Statement{nested}), true},
		{"return alternate", branch([]statements.Statement{ret}, []statements.Statement{nested}), true},
		{"two nested normal exits", branch([]statements.Statement{nested}, []statements.Statement{nested}), true},
		{"nested conditional", branch([]statements.Statement{br}, []statements.Statement{branch([]statements.Statement{nested}, []statements.Statement{ret})}), true},
		{"implicit else fallthrough", branch([]statements.Statement{nested}, nil), false},
		{"unrelated normal alternate", branch([]statements.Statement{nested}, []statements.Statement{effect}), false},
		{"statement after nested", branch([]statements.Statement{nested, effect}, []statements.Statement{ret}), false},
		{"returning nested switch", terminal, false},
		{"both paths abrupt", branch([]statements.Statement{br}, []statements.Statement{ret}), false},
		{"unreachable nested switch", branch([]statements.Statement{ret, nested}, []statements.Statement{br}), false},
		{"loop transfer", statements.NewSourceTransferStatement("continue", ""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sharedSwitchTailCompletes(tc.tail); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
