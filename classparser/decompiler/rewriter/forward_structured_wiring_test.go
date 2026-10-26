package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestForwardRegionRetainsBypassAfterFallbackStructuring(t *testing.T) {
	m, n := forwardRegionFixture()
	owner, inner, work, exit := n[1], n[2], n[5], n[6]
	truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	exit.Statement = &statements.ConditionStatement{Condition: truth}
	exit.RemoveAllNext()
	end := m.NewNode(&statements.MiddleStatement{Flag: "end"})
	for i := 0; i < 2; i++ {
		pc := 70 + i*10
		ret := m.NewNode(&statements.ReturnStatement{JavaValue: values.NewJavaLiteral(i+1, types.NewJavaPrimer(types.JavaInteger)), HasOriginPC: true, OriginPC: pc})
		ret.HasOriginPC, ret.OriginPC = true, pc
		ret.AddNext(end)
		exit.AddNext(ret)
	}
	exit.TrueNode = func() *core.Node { return exit.Next[0] }
	exit.FalseNode = func() *core.Node { return exit.Next[1] }
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	if err := IfRewriter(m, exit); err != nil {
		t.Fatal(err)
	}
	replacement := work.Next[0]
	if _, ok := replacement.Statement.(*statements.IfStatement); !ok || replacement == exit || replacement.HasOriginPC {
		t.Fatal("expected a synthesized fallback decision")
	}
	m.DominatorMap = GenerateDominatorTree(m.RootNode)
	if !closedForwardRegion(m, owner, replacement) {
		t.Fatal("original independent exit lost after structuring")
	}
	materializeForwardRegionExits(m, inner)
	region := m.forwardRegions[owner]
	if region == nil || region.exit != replacement {
		t.Fatal("lost original bypass")
	}
}
