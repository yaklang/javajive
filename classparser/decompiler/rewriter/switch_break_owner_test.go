package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSwitchCompletionRequiresItsOwnBreak(t *testing.T) {
	br := statements.NewSourceTransferStatement("break", "")
	ret := statements.NewReturnStatement(nil)
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	inner := statements.NewSwitchStatement(values.JavaNull, []*statements.CaseItem{{IsDefault: true, Body: []statements.Statement{br}}})
	loop := statements.NewDoWhileStatement(truth, []statements.Statement{br})
	conditional := statements.NewIfStatement(values.JavaNull, []statements.Statement{br}, nil)
	for _, tc := range []struct {
		name string
		body []statements.Statement
		want bool
	}{
		{"own break", []statements.Statement{br}, true},
		{"conditional own break before return", []statements.Statement{conditional, ret}, true},
		{"inner switch break before return", []statements.Statement{inner, ret}, false},
		{"inner loop break before return", []statements.Statement{loop, ret}, false},
		{"loop then own break", []statements.Statement{loop, br}, true},
		{"switch then own break", []statements.Statement{inner, br}, true},
		{"labelled outer break", []statements.Statement{statements.NewSourceTransferStatement("break", "OUTER")}, false},
		{"labelled outer continue", []statements.Statement{statements.NewSourceTransferStatement("continue", "OUTER")}, false},
		{"unreachable own break", []statements.Statement{ret, br}, false},
		{"last inner switch falls out", []statements.Statement{inner}, true},
		{"empty default", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sw := statements.NewSwitchStatement(values.JavaNull, []*statements.CaseItem{{IsDefault: true, Body: tc.body}})
			if got := switchCompletesNormally(sw); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
