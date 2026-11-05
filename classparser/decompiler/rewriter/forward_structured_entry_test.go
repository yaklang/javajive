package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestForwardRegionStructuredExitKeepsOriginalIncomingBoundary(t *testing.T) {
	for _, kind := range []string{"proved", "renumbered", "missing witness", "missing original", "invalid origin", "not decision", "opaque replacement", "missing condition", "stale statement", "changed domain", "catch entry", "hidden entry", "budget"} {
		t.Run(kind, func(t *testing.T) {
			m, n := forwardRegionFixture()
			owner, inner, original := n[1], n[2], n[6]
			truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			original.Statement = &statements.ConditionStatement{Condition: truth}
			statement := statements.NewIfStatement(truth, nil, nil)
			replacement := m.NewNode(statement)
			replacement.AddNext(n[7])
			original.Replace(replacement)
			switch kind {
			case "missing original":
				original.HasOriginPC = false
			case "invalid origin":
				original.OriginPC = -1
			case "not decision":
				original.Statement = statements.NewExpressionStatement(truth)
			case "opaque replacement":
				replacement.Statement = &statements.CustomStatement{}
			case "missing condition":
				statement.Condition = nil
			case "catch entry":
				original.IsCatchStart = true
			case "hidden entry":
				original.HideNext = n[7]
			case "budget":
				m.forwardEntries = map[*core.Node]forwardEntry{}
				for i := 0; i < 4096; i++ {
					m.forwardEntries[core.NewNode(nil)] = forwardEntry{}
				}
			}
			if kind != "missing witness" {
				rememberForwardEntry(m, original, replacement)
			}
			if kind == "renumbered" {
				replacement.Id = 999
				original.OriginPC = 1000
			}
			if kind == "stale statement" {
				replacement.Statement = statements.NewIfStatement(truth, nil, nil)
			}
			if kind == "changed domain" {
				m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 60, EndPc: 61, HandlerPc: 90, CatchType: 1}})
			}
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
			want := kind == "proved" || kind == "renumbered"
			if got := closedForwardRegion(m, owner, replacement); got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
			if replacement.HasOriginPC {
				t.Fatal("synthetic wrapper acquired a physical opcode witness")
			}
			materializeForwardRegionExits(m, inner)
			if got := m.forwardRegions[owner]; (got != nil) != want {
				t.Fatalf("bypass proof=%v want=%v", got, want)
			} else if got != nil && got.exit != replacement {
				t.Fatal("changed bypass destination")
			}
		})
	}
}
