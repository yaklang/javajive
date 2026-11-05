package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"slices"
	"testing"
)

func TestSharedTypedResumeRequiresCompletePacketAndNormalFrontier(t *testing.T) {
	for _, variant := range []string{"two handlers", "one handler", "both resume", "both terminate", "missing table", "missing row", "extra interval", "competing handler", "catchall row", "changed type", "invalid range", "different catch ranges", "catchall", "wrong handler PC", "missing handler PC", "duplicate target", "stale boundary", "unprotected body effect", "protected update effect", "unknown update PC", "hidden end", "encoded end", "cyclic jump", "hidden jump", "third normal frontier", "oversized table"} {
		t.Run(variant, func(t *testing.T) {
			m, n := forwardRegionFixture()
			root, start, work, end := n[0], n[1], n[5], n[6]
			owner := m.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			owner.HasProtectedRange = true
			owner.ProtectedStartPC, owner.ProtectedEndPC = 10, 25
			ranges := [][2]int{{10, 25}, {30, 55}}
			first := m.NewNode(statements.NewExpressionStatement(layerTestCall(80)))
			first.IsCatchStart, first.HasOriginPC, first.OriginPC = true, true, 80
			first.CatchHandler = &statements.CatchHandler{EntryPC: 80, ProtectedRanges: slices.Clone(ranges)}
			second := m.NewNode(&statements.ReturnStatement{HasOriginPC: true, OriginPC: 90})
			second.IsCatchStart, second.HasOriginPC, second.OriginPC = true, true, 90
			second.CatchHandler = &statements.CatchHandler{EntryPC: 90, ProtectedRanges: slices.Clone(ranges)}
			root.ReplaceNext(start, owner)
			owner.AddNext(start)
			owner.AddNext(first)
			owner.AddNext(second)
			first.AddNext(end)
			jump := m.NewNode(&statements.GOTOStatement{})
			jump.HasOriginPC, jump.OriginPC = true, 25
			jump.AddNext(end)
			owner.ProtectedEnd = jump
			m.TryNodes = []*core.Node{owner}
			m.originalHandlers = []core.HandlerRange{{StartPc: 10, EndPc: 25, HandlerPc: 80, CatchType: 1}, {StartPc: 30, EndPc: 55, HandlerPc: 80, CatchType: 1}, {StartPc: 10, EndPc: 25, HandlerPc: 90, CatchType: 2}, {StartPc: 30, EndPc: 55, HandlerPc: 90, CatchType: 2}}
			end.Statement = statements.NewExpressionStatement(layerTestCall(60))
			want := variant == "two handlers" || variant == "one handler" || variant == "both resume"
			switch variant {
			case "one handler":
				owner.RemoveNext(second)
				m.originalHandlers = m.originalHandlers[:2]
			case "both resume":
				second.Statement = statements.NewExpressionStatement(layerTestCall(90))
				second.AddNext(end)
			case "both terminate":
				first.RemoveNext(end)
				first.Statement = &statements.ReturnStatement{}
			case "missing table":
				m.originalHandlersKnown = false
			case "missing row":
				m.originalHandlers = m.originalHandlers[:3]
			case "extra interval":
				m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 60, EndPc: 65, HandlerPc: 80, CatchType: 1})
			case "competing handler":
				m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 15, EndPc: 20, HandlerPc: 95, CatchType: 3})
			case "catchall row":
				m.originalHandlers[1].CatchType = 0
			case "changed type":
				m.originalHandlers[1].CatchType = 3
			case "invalid range":
				m.originalHandlers[1].StartPc = 60
			case "different catch ranges":
				second.CatchHandler.ProtectedRanges[1][1]++
			case "catchall":
				first.CatchHandler.CatchAll = true
			case "wrong handler PC":
				first.CatchHandler.EntryPC++
			case "missing handler PC":
				first.HasOriginPC = false
			case "duplicate target":
				second.OriginPC = 80
				second.CatchHandler.EntryPC = 80
			case "stale boundary":
				owner.ProtectedEnd = m.NewNode(statements.NewExpressionStatement(layerTestCall(60)))
			case "unprotected body effect":
				work.Statement = statements.NewExpressionStatement(layerTestCall(60))
			case "protected update effect":
				end.Statement = statements.NewExpressionStatement(layerTestCall(40))
			case "unknown update PC":
				end.HasOriginPC = false
			case "hidden end":
				end.HideNext = first
			case "encoded end":
				end.EncodedJumps = map[*core.Node]bool{first: true}
			case "cyclic jump":
				jump.ReplaceNext(end, jump)
			case "hidden jump":
				jump.HideNext = first
			case "third normal frontier":
				other := m.NewNode(statements.NewExpressionStatement(layerTestCall(70)))
				other.HasOriginPC, other.OriginPC = true, 70
				n[2].AddNext(other)
			case "oversized table":
				m.originalHandlers = make([]core.HandlerRange, 513)
			}
			m.DominatorMap = GenerateDominatorTree(root)
			before := slices.Clone(owner.Next)
			got := typedSharedResumeBoundary(m, owner, start)
			if (got == end) != want || !want && got != nil {
				t.Fatalf("frontier=%p want=%v", got, want)
			}
			if !slices.Equal(owner.Next, before) || first.CatchHandler == nil {
				t.Fatal("proof changed graph")
			}
		})
	}
}
