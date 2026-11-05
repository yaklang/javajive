package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestForwardSplitHandlerRequiresExactDispatchAndOutsideBoundary(t *testing.T) {
	for _, variant := range []string{"split handler", "reordered disjoint rows", "missing shared packet", "wrong shared target", "wrong shared type", "catchall row", "missing original interval", "missing catch origin", "stale catch origin", "wrong catch range", "oversized packet", "invalid interval", "stale boundary", "protected boundary operand", "nonterminal handler", "interior gap", "different dispatch target", "different dispatch type", "different dispatch priority", "foreign entry", "unknown original table"} {
		t.Run(variant, func(t *testing.T) {
			m, nodes := forwardRegionFixture()
			root, owner, inner, work, exit := nodes[0], nodes[1], nodes[2], nodes[5], nodes[6]
			region := m.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange, region.SharedProtectedHandler = true, true
			region.ProtectedStartPC, region.ProtectedEndPC = 10, 25
			rows := []core.HandlerRange{{StartPc: 10, EndPc: 25, HandlerPc: 90, CatchType: 7}, {StartPc: 30, EndPc: 55, HandlerPc: 90, CatchType: 7}}
			region.SharedProtectedRanges = slices.Clone(rows)
			m.originalHandlers = slices.Clone(rows)
			handler := m.NewNode(&statements.ReturnStatement{OriginPC: 90, HasOriginPC: true})
			handler.OriginPC, handler.HasOriginPC, handler.IsCatchStart = 90, true, true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 90, ProtectedRanges: [][2]int{{10, 25}, {30, 55}}}
			jump := m.NewNode(&statements.GOTOStatement{})
			jump.OriginPC, jump.HasOriginPC = 25, true
			jump.AddNext(exit)
			region.ProtectedEnd = jump
			replaceNextInPlace(root, owner, region)
			region.AddNext(owner)
			region.AddNext(handler)
			exit.Statement = statements.NewExpressionStatement(layerTestCall(60))
			m.TryNodes = []*core.Node{region}
			want := variant == "split handler" || variant == "reordered disjoint rows"
			switch variant {
			case "reordered disjoint rows":
				m.originalHandlers[0], m.originalHandlers[1] = m.originalHandlers[1], m.originalHandlers[0]
			case "missing shared packet":
				region.SharedProtectedRanges = nil
			case "wrong shared target":
				region.SharedProtectedRanges[1].HandlerPc++
			case "wrong shared type":
				region.SharedProtectedRanges[1].CatchType++
			case "catchall row":
				region.SharedProtectedRanges[1].CatchType = 0
			case "missing original interval":
				region.ProtectedStartPC++
			case "missing catch origin":
				handler.HasOriginPC = false
			case "stale catch origin":
				handler.OriginPC++
			case "wrong catch range":
				handler.CatchHandler.ProtectedRanges[1][1]++
			case "oversized packet":
				region.SharedProtectedRanges = make([]core.HandlerRange, 33)
			case "invalid interval":
				m.originalHandlers[1].EndPc = 29
			case "stale boundary":
				region.ProtectedEnd = m.NewNode(statements.NewExpressionStatement(layerTestCall(60)))
			case "protected boundary operand":
				exit.Statement = statements.NewExpressionStatement(layerTestCall(40))
			case "nonterminal handler":
				handler.Statement = &statements.MiddleStatement{}
			case "interior gap":
				work.OriginPC = 26
			case "different dispatch target":
				m.originalHandlers[1].HandlerPc++
			case "different dispatch type":
				m.originalHandlers[1].CatchType++
			case "different dispatch priority":
				m.originalHandlers = []core.HandlerRange{rows[0], {StartPc: 10, EndPc: 55, HandlerPc: 99, CatchType: 8}, rows[1]}
			case "foreign entry":
				root.AddNext(work)
			case "unknown original table":
				m.originalHandlersKnown = false
			}
			m.DominatorMap = GenerateDominatorTree(root)
			before := map[*core.Node][]*core.Node{}
			for _, n := range WalkNodeToList(root) {
				before[n] = slices.Clone(n.Next)
			}
			if want && !typedProtectedIfCollectionStops(m, owner)[exit] {
				t.Fatal("raw protected region has no closed outside boundary")
			}
			materializeForwardRegionExits(m, inner)
			got := m.forwardRegions[owner]
			if (got != nil) != want {
				t.Fatalf("accepted=%v want=%v", got != nil, want)
			}
			if !want {
				for n, next := range before {
					if !slices.Equal(n.Next, next) {
						t.Fatal("rejected proof changed graph")
					}
				}
				return
			}
			if got.exit != exit {
				t.Fatal("block and try disagree on outside continuation")
			}
			for _, n := range []*core.Node{inner, owner} {
				m.DominatorMap = GenerateDominatorTree(root)
				if err := IfRewriter(m, n); err != nil {
					t.Fatal(err)
				}
			}
			block, ok := region.Next[0].Statement.(*statements.DoWhileStatement)
			if !ok || len(region.Next[0].Next) != 1 || region.Next[0].Next[0] != exit {
				t.Fatal("outside effect moved into protected block")
			}
			for _, st := range block.Body {
				if st == exit.Statement {
					t.Fatal("outside effect was copied")
				}
			}
		})
	}
}

func TestSplitHandlerBoundaryRequiresEveryNormalPath(t *testing.T) {
	for _, variant := range []string{"closed", "gap jump", "outside commit", "outside folded effect", "cycle", "unknown effect", "hidden edge", "encoded edge", "unproved terminal", "pending handler", "second frontier", "oversized prefix", "outside swallowed nested effect"} {
		t.Run(variant, func(t *testing.T) {
			m, n := forwardRegionFixture()
			start, work, end := n[1], n[5], n[6]
			ranges := [][2]int{{10, 25}, {30, 55}}
			want := variant == "closed" || variant == "gap jump"
			switch variant {
			case "gap jump":
				jump := m.NewNode(&statements.GOTOStatement{})
				jump.HasOriginPC, jump.OriginPC = true, 25
				replaceNextInPlace(n[2], end, jump)
				jump.AddNext(end)
			case "outside commit":
				work.OriginPC = 26
				work.Statement = statements.NewExpressionStatement(layerTestCall(26))
			case "outside folded effect":
				work.Statement = statements.NewExpressionStatement(layerTestCall(60))
			case "cycle":
				work.ReplaceNext(end, start)
			case "unknown effect":
				call := layerTestCall(40)
				call.HasOriginPC = false
				work.Statement = statements.NewExpressionStatement(call)
			case "hidden edge":
				work.HideNext = end
			case "encoded edge":
				work.EncodedJumps = map[*core.Node]bool{end: true}
			case "unproved terminal":
				work.RemoveNext(end)
				work.Statement = &statements.CustomStatement{}
			case "pending handler":
				work.IsTryCatch = true
			case "second frontier":
				n[2].AddNext(core.NewNode(statements.NewExpressionStatement(layerTestCall(70))))
			case "outside swallowed nested effect":
				work.Statement = &statements.TryCatchStatement{TryBody: []statements.Statement{statements.NewExpressionStatement(layerTestCall(65))}, Exception: []*values.JavaRef{layerTestRef("java.lang.Throwable")}, CatchBodies: [][]statements.Statement{{statements.NewAssignStatement(layerTestRef("java.lang.Object"), values.JavaNull, false)}}, Handlers: []statements.CatchHandler{{EntryPC: 75, ProtectedRanges: [][2]int{{60, 70}}}}}
			case "oversized prefix":
				work.RemoveNext(end)
				cursor := work
				for i := 0; i < 512; i++ {
					next := m.NewNode(statements.NewExpressionStatement(layerTestCall(40)))
					cursor.AddNext(next)
					cursor = next
				}
				cursor.AddNext(end)
			}
			if got := typedSplitNormalBoundaryClosed(start, end, ranges); got != want {
				t.Fatalf("closed=%v want=%v", got, want)
			}
		})
	}
}
