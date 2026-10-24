package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/omap"
)

func TestSwitchSharedTransferRetainsEveryPhysicalEntry(t *testing.T) {
	for _, kind := range []string{"continue", "break"} {
		t.Run(kind, func(t *testing.T) {
			root := jumpTestNode("root")
			indices := omap.NewEmptyOrderedMap[int, int]()
			offsets := omap.NewEmptyOrderedMap[int, int32]()
			for i, offset := range []int32{10, 20, 30} {
				indices.Set(i+1, i)
				offsets.Set(i+1, offset)
			}
			selector := values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch", Data: []any{indices, selector, offsets, statements.SwitchDefault{Index: 1, Offset: 40}}})
			owner.SwitchPrepared = true
			first := core.NewNode(&statements.ReturnStatement{JavaValue: values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))})
			transfer := core.NewNode(statements.NewSourceTransferStatement(kind, ""))
			third := core.NewNode(&statements.ReturnStatement{JavaValue: values.NewJavaLiteral(-1, types.NewJavaPrimer(types.JavaInteger))})
			end := core.NewNode(&statements.MiddleStatement{Flag: "end"})
			root.AddNext(owner)
			owner.AddNext(first)
			owner.AddNext(transfer)
			owner.AddNext(third)
			for _, n := range []*core.Node{first, transfer, third} {
				n.AddNext(end)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			if err := SwitchRewriter(manager, owner); err != nil {
				t.Fatal(err)
			}
			sw, ok := root.Next[0].Statement.(*statements.SwitchStatement)
			if !ok || len(sw.Cases) != 4 {
				t.Fatal("complete original switch labels were lost")
			}
			for _, arm := range sw.Cases {
				if !arm.IsDefault && arm.IntValue != 2 {
					continue
				}
				if len(arm.Body) != 1 || arm.Body[0] != transfer.Statement {
					t.Fatalf("physical label %d/default=%v lost its own %s edge", arm.IntValue, arm.IsDefault, kind)
				}
			}
		})
	}
}

func TestSwitchTransferGroupingRequiresEntryIdentityAndSealedProtocol(t *testing.T) {
	labels := []switchLabel{{Value: -1}, {Default: true}}
	for _, variant := range []string{"distinct", "same", "missing", "labeled", "opaque", "effect", "return", "throw"} {
		t.Run(variant, func(t *testing.T) {
			target := core.NewNode(statements.NewSourceTransferStatement("continue", ""))
			offsets := map[switchLabel]int{labels[0]: 10, labels[1]: 30}
			switch variant {
			case "same":
				offsets[labels[1]] = 10
			case "missing":
				delete(offsets, labels[1])
			case "labeled":
				target.Statement = statements.NewSourceTransferStatement("continue", "outer")
			case "opaque":
				target.Statement = &statements.CustomStatement{StringFunc: func(*class_context.ClassContext) string {
					t.Fatal("grouping rendered opaque source")
					return "continue"
				}}
			case "effect":
				target = jumpTestNode("observableEffect()")
			case "return":
				target.Statement = &statements.ReturnStatement{}
			case "throw":
				target = jumpTestNode("throw new IllegalStateException()")
			}
			got := switchTransferEntriesNeedOwnBody(target, labels, offsets)
			if got != (variant == "distinct" || variant == "missing" || variant == "labeled") {
				t.Fatal("entry/transfer classification", got)
			}
		})
	}
}
