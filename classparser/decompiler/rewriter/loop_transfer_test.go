package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

func TestNestedLoopTransferKeepsOriginalTargetAndEdges(t *testing.T) {
	for _, scenario := range []string{"continue", "break", "same owner", "sibling", "not dominated", "no witness", "opaque", "throw", "not loop"} {
		t.Run(scenario, func(t *testing.T) {
			outer := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			inner := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			jump := core.NewNode(&statements.CustomStatement{Name: "continue", StringFunc: func(*class_context.ClassContext) string { return "continue" }, OriginPC: 17, HasOriginPC: true})
			outer.AddNext(inner)
			inner.AddNext(jump)
			jump.AddNext(outer)
			manager := NewRootStatementManager(outer)
			manager.DominatorMap = GenerateDominatorTree(outer)
			kind := "continue"
			if scenario == "break" {
				kind = "break"
			}
			manager.recordLoopTransfer(jump, outer, kind)
			current := inner
			switch scenario {
			case "same owner":
				current = outer
			case "sibling":
				delete(manager.DominatorMap, outer)
			case "not dominated":
				delete(manager.DominatorMap, inner)
			case "no witness":
				delete(manager.loopTransfers, jump)
			case "opaque":
				jump.Statement = &statements.MiddleStatement{}
			case "throw":
				jump.Statement.(*statements.CustomStatement).ThrownValue = values.JavaNull
			case "not loop":
				outer.Statement = &statements.MiddleStatement{}
			}
			original := jump.Statement
			manager.qualifyLoopTransfer(jump, current)
			want := scenario == "continue" || scenario == "break"
			if (jump.Statement != original) != want {
				t.Fatal("incorrect transfer qualification")
			}
			if want {
				st := jump.Statement.(*statements.CustomStatement)
				label := outer.Statement.(*statements.DoWhileStatement).Label
				if label == "" || !strings.Contains(st.StringFunc(nil), kind+" "+label) || !st.HasOriginPC || st.OriginPC != 17 {
					t.Fatal("lost target or origin")
				}
			}
			if len(jump.Next) != 1 || jump.Next[0] != outer || len(inner.Next) != 1 || inner.Next[0] != jump {
				t.Fatal("rewired transfer")
			}
		})
	}
}

func TestOuterTransferIsQualifiedBeforeContainerCapture(t *testing.T) {
	outer := core.NewNode(statements.NewDoWhileStatement(nil, nil))
	inner := core.NewNode(statements.NewDoWhileStatement(nil, nil))
	source := core.NewNode(&statements.ConditionStatement{})
	outer.AddNext(inner)
	inner.AddNext(source)
	source.AddNext(outer)
	manager := NewRootStatementManager(outer)
	manager.WhileNode = []*core.Node{outer, inner}
	manager.DominatorMap = GenerateDominatorTree(outer)
	transfer := core.NewNode(&statements.CustomStatement{Name: "continue", StringFunc: func(*class_context.ClassContext) string { return "continue" }})
	manager.recordLoopTransfer(transfer, outer, "continue", source)
	captured := transfer.Statement.(*statements.CustomStatement)
	label := outer.Statement.(*statements.DoWhileStatement).Label
	if label == "" || captured.Name != "" || captured.StringFunc(nil) != "continue "+label {
		t.Fatal("container would capture an unqualified outer transfer")
	}
	if len(transfer.Next) != 0 || source.Next[0] != outer {
		t.Fatal("changed original CFG target")
	}
}
