package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

// A cyclic body can terminate in a head throw or a later return. Neither path
// resumes the method after the loop. A normal shared continuation, on the other
// hand, must remain reachable; foreign incoming edges and missing typed origins
// cannot be treated as an exclusively owned terminal arm.
func TestLoopHeaderOriginalThrowIsAbruptNotNormalContinuation(t *testing.T) {
	for _, scenario := range []string{"private throw", "sole throw", "normal continuation", "shared entry", "no node origin", "no operand origin", "different origin", "opaque text", "catch entry", "nonlocal successor", "source transfer", "irreducible"} {
		t.Run(scenario, func(t *testing.T) {
			entry := core.NewNode(&statements.ConditionStatement{})
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			header := core.NewNode(&statements.ConditionStatement{})
			body := core.NewNode(&statements.ConditionStatement{})
			latch := core.NewNode(&statements.ExpressionStatement{})
			ret := core.NewNode(&statements.ReturnStatement{})
			thrown := core.NewNode(&statements.CustomStatement{ThrownValue: values.JavaNull, HasOriginPC: true, OriginPC: 17, StringFunc: func(*class_context.ClassContext) string { return "throw null" }})
			thrown.HasOriginPC, thrown.OriginPC = true, 17
			entry.AddNext(loop)
			loop.AddNext(header)
			header.AddNext(thrown)
			header.AddNext(body)
			body.AddNext(ret)
			body.AddNext(latch)
			latch.AddNext(loop)
			reducible := true
			var want *core.Node
			switch scenario {
			case "sole throw":
				body.RemoveNext(ret)
			case "normal continuation":
				ret.Statement = &statements.ExpressionStatement{}
				tail := core.NewNode(&statements.ReturnStatement{})
				ret.AddNext(tail)
				entry.AddNext(ret)
				want = ret
			case "shared entry":
				entry.AddNext(thrown)
				want = thrown
			case "no node origin":
				thrown.HasOriginPC = false
				want = thrown
			case "no operand origin":
				thrown.Statement.(*statements.CustomStatement).HasOriginPC = false
				want = thrown
			case "different origin":
				thrown.Statement.(*statements.CustomStatement).OriginPC = 18
				want = thrown
			case "opaque text":
				thrown.Statement.(*statements.CustomStatement).ThrownValue = nil
				want = thrown
			case "catch entry":
				thrown.IsCatchStart = true // exceptional entries are not normal exits
				want = ret
			case "nonlocal successor":
				thrown.AddNext(loop)
				want = ret
			case "source transfer":
				thrown.Statement = statements.NewSourceTransferStatement("break", "")
				want = thrown
			case "irreducible":
				reducible = false // existing conservative multi-exit analysis applies
				want = nil
			}
			dom := GenerateDominatorTree(entry)
			got := searchCircleEndNode(loop, header, dom, reducible)
			if got != want {
				t.Fatalf("normal continuation=%p want=%p", got, want)
			}
		})
	}
}
