package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestAdversarialNestedNormalContinuations(t *testing.T) {
	for _, scenario := range []string{"outer fallback", "method end", "loop test", "monitor exit", "catch parent", "switch fallthrough", "switch end", "only marker follows"} {
		t.Run(scenario, func(t *testing.T) {
			leaf := statements.NewTryCatchStatement(nil, [][]statements.Statement{nil})
			arm := &statements.IfStatement{IfBody: []statements.Statement{leaf}}
			outer := &statements.IfStatement{ElseBody: []statements.Statement{arm}}
			tail := &statements.ReturnStatement{}
			body := []statements.Statement{outer, tail}
			want := true
			switch scenario {
			case "method end":
				body = body[:1]
				want = false
			case "loop test":
				body = []statements.Statement{statements.NewDoWhileStatement(nil, []statements.Statement{outer})}
			case "monitor exit":
				body = []statements.Statement{&statements.SynchronizedStatement{Body: []statements.Statement{outer}}, tail}
			case "catch parent":
				body = []statements.Statement{statements.NewTryCatchStatement(nil, [][]statements.Statement{{outer}}), tail}
			case "switch fallthrough":
				body = []statements.Statement{&statements.SwitchStatement{Cases: []*statements.CaseItem{{Body: []statements.Statement{outer}}, {Body: []statements.Statement{tail}}}}}
			case "switch end":
				body = []statements.Statement{&statements.SwitchStatement{Cases: []*statements.CaseItem{{Body: []statements.Statement{outer}}}}}
				want = false
			case "only marker follows":
				body[1] = &statements.MiddleStatement{Flag: "end"}
				want = false
			}
			if got := normalStatementContinuations(body)[leaf]; got != want {
				t.Fatalf("catch has normal continuation=%v want=%v", got, want)
			}
		})
	}
}
