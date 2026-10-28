package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
	"testing"
)

func TestProtectedCommitExitRequiresClosedPhysicalDomains(t *testing.T) {
	for _, variant := range []string{"single interval", "split interval", "swapped polarity", "missing table", "missing physical row", "extra physical row", "wrong physical target", "wrong physical type", "overlap priority", "catchall", "wrong packet", "wrong handler PC", "handler resumes", "foreign prefix entry", "foreign commit entry", "foreign tail entry", "unknown commit effect", "protected commit operand", "protected update operand", "hidden prefix", "encoded prefix", "prefix cycle", "tail cycle", "third frontier", "nonlinear tail", "unknown update PC", "missing condition PC", "oversized tail", "oversized table", "wrong owner marker", "hidden handler", "encoded handler", "hidden foreign prefix", "hidden foreign tail"} {
		t.Run(variant, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{})
			owner := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			owner.HasProtectedRange = true
			owner.ProtectedStartPC, owner.ProtectedEndPC = 10, 30
			condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			condition.HasOriginPC, condition.OriginPC = true, 20
			store := core.NewNode(statements.NewAssignStatement(layerTestRef("java.lang.Object"), values.JavaNull, false))
			store.HasOriginPC, store.OriginPC = true, 25
			commit := core.NewNode(statements.NewExpressionStatement(layerTestCall(40)))
			commit.HasOriginPC, commit.OriginPC = true, 40
			tail := core.NewNode(statements.NewExpressionStatement(layerTestCall(45)))
			tail.HasOriginPC, tail.OriginPC = true, 45
			update := core.NewNode(statements.NewExpressionStatement(layerTestCall(50)))
			update.HasOriginPC, update.OriginPC = true, 50
			handler := core.NewNode(&statements.ReturnStatement{HasOriginPC: true, OriginPC: 80})
			handler.HasOriginPC, handler.OriginPC, handler.IsCatchStart = true, 80, true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 80, ProtectedRanges: [][2]int{{10, 30}}}
			root.AddNext(owner)
			owner.AddNext(condition)
			owner.AddNext(handler)
			condition.AddNext(store)
			condition.AddNext(update)
			store.AddNext(commit)
			commit.AddNext(tail)
			tail.AddNext(update)
			update.AddNext(owner)
			condition.TrueNode = func() *core.Node { return condition.Next[0] }
			condition.FalseNode = func() *core.Node { return condition.Next[1] }
			owner.ProtectedEnd = update
			m := NewRootStatementManager(root)
			m.TryNodes = []*core.Node{owner}
			m.currentNodeId = 100
			m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 10, EndPc: 30, HandlerPc: 80, CatchType: 7}})
			want := variant == "single interval" || variant == "split interval" || variant == "swapped polarity"
			switch variant {
			case "split interval":
				owner.ProtectedEndPC = 23
				owner.SharedProtectedHandler = true
				owner.SharedProtectedRanges = []core.HandlerRange{{StartPc: 10, EndPc: 23, HandlerPc: 80, CatchType: 7}, {StartPc: 24, EndPc: 30, HandlerPc: 80, CatchType: 7}}
				handler.CatchHandler.ProtectedRanges = [][2]int{{10, 23}, {24, 30}}
				m.originalHandlers = slices.Clone(owner.SharedProtectedRanges)
			case "swapped polarity":
				condition.Next[0], condition.Next[1] = condition.Next[1], condition.Next[0]
			case "wrong owner marker":
				owner.Statement = &statements.MiddleStatement{}
			case "hidden handler":
				handler.HideNext = update
			case "encoded handler":
				handler.EncodedJumps = map[*core.Node]bool{update: true}
			case "hidden foreign prefix":
				root.HideNext = store
			case "hidden foreign tail":
				root.HideNext = tail
			case "missing table":
				m.originalHandlersKnown = false
			case "missing physical row":
				m.originalHandlers = nil
			case "extra physical row":
				m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 40, EndPc: 46, HandlerPc: 80, CatchType: 7})
			case "wrong physical target":
				m.originalHandlers[0].HandlerPc++
			case "wrong physical type":
				m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 10, EndPc: 30, HandlerPc: 80, CatchType: 9})
			case "overlap priority":
				m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 20, EndPc: 26, HandlerPc: 90, CatchType: 8})
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "wrong packet":
				handler.CatchHandler.ProtectedRanges[0][1]++
			case "wrong handler PC":
				handler.CatchHandler.EntryPC++
			case "handler resumes":
				handler.Statement = &statements.MiddleStatement{}
				handler.AddNext(update)
			case "foreign prefix entry":
				root.AddNext(store)
			case "foreign commit entry":
				root.AddNext(commit)
			case "foreign tail entry":
				root.AddNext(tail)
			case "unknown commit effect":
				commit.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "protected commit operand":
				commit.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 25
			case "protected update operand":
				update.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 25
			case "hidden prefix":
				condition.HideNext = update
			case "encoded prefix":
				condition.EncodedJumps = map[*core.Node]bool{update: true}
			case "prefix cycle":
				store.ReplaceNext(commit, condition)
			case "tail cycle":
				tail.ReplaceNext(update, commit)
			case "third frontier":
				other := core.NewNode(statements.NewExpressionStatement(layerTestCall(60)))
				other.HasOriginPC, other.OriginPC = true, 60
				condition.AddNext(other)
			case "nonlinear tail":
				tail.AddNext(core.NewNode(&statements.ReturnStatement{}))
			case "unknown update PC":
				update.HasOriginPC = false
			case "missing condition PC":
				condition.HasOriginPC = false
			case "oversized tail":
				tail.RemoveNext(update)
				cursor := tail
				for i := 0; i < 128; i++ {
					n := core.NewNode(statements.NewExpressionStatement(layerTestCall(45)))
					n.HasOriginPC, n.OriginPC = true, 45
					cursor.AddNext(n)
					cursor = n
				}
				cursor.AddNext(update)
			case "oversized table":
				m.originalHandlers = make([]core.HandlerRange, 513)
			}
			m.DominatorMap = GenerateDominatorTree(root)
			before := map[*core.Node][]*core.Node{}
			for _, n := range WalkNodeToList(root) {
				before[n] = slices.Clone(n.Next)
			}
			materializeProtectedCommitExit(m, condition)
			region := m.forwardRegions[owner]
			if (region != nil) != want {
				t.Fatalf("planned=%v want=%v", region != nil, want)
			}
			if !want {
				for n, next := range before {
					if !slices.Equal(n.Next, next) {
						t.Fatal("rejection mutated graph")
					}
				}
				return
			}
			if region.exit != update || region.protectedEnd != commit || store.Next[0] != commit || commit.Next[0] != tail || tail.Next[0] != update || owner.Next[1] != handler || !typedProtectedIfCollectionStops(m, condition)[commit] {
				t.Fatal("lost original protected/outside boundaries")
			}
			index := 1
			if variant == "swapped polarity" {
				index = 0
			}
			jump := condition.Next[index]
			leaf, ok := jump.Statement.(*statements.CustomStatement)
			if !ok {
				t.Fatal("missing break")
			}
			kind, label, known := leaf.SourceTransferProtocol()
			if !known || kind != "break" || label != region.label || !jump.EncodedJumps[update] {
				t.Fatal("lost skip destination")
			}
			materializeProtectedCommitExit(m, condition)
			if condition.Next[index] != jump {
				t.Fatal("duplicate planning")
			}
		})
	}
}
