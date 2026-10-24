package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestTypedLoopTryFrontierRequiresExactTransferAndWholeNormalDomain(t *testing.T) {
	variants := []string{"original", "structured branch", "pure return", "no physical end", "wrong end PC", "nonjump end", "ambiguous end", "foreign header", "missing header origin", "unknown loop", "wrong recorded kind", "wrong recorded target", "opaque transfer", "changed sealed kind", "changed label", "changed live target", "catchall", "multiple handlers", "nonterminal handler", "protected return operand", "mixed return operand", "missing operand origin", "missing return origin", "mismatched return PC", "return with live successor", "unmatched encoded edge", "unprotected prefix effect", "unprotected predicate", "invalid predicate edges", "opaque prefix", "two normal returns", "stale dominators"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			owner := core.NewNode(statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil))
			header := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			header.OriginPC, header.HasOriginPC = 2, true
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange, region.ProtectedStartPC, region.ProtectedEndPC = true, 10, 20
			condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			condition.OriginPC, condition.HasOriginPC = 14, true
			jump := core.NewNode(&statements.GOTOStatement{})
			jump.OriginPC, jump.HasOriginPC = 20, true
			jump.AddNext(header)
			region.ProtectedEnd = jump
			leaf := statements.NewSourceTransferStatement("continue", "")
			transfer := core.NewNode(leaf)
			transfer.IsJmp = true
			transfer.AddNext(owner)
			call := layerTestCall(48)
			ret := &statements.ReturnStatement{JavaValue: call, OriginPC: 51, HasOriginPC: true}
			tail := core.NewNode(ret)
			tail.OriginPC, tail.HasOriginPC = 51, true
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 25, ProtectedRanges: [][2]int{{10, 20}}}
			owner.AddNext(header)
			header.AddNext(region)
			region.AddNext(condition)
			region.AddNext(handler)
			condition.AddNext(transfer)
			condition.AddNext(tail)
			manager := NewRootStatementManager(owner)
			manager.WhileNode = []*core.Node{owner}
			manager.recordLoopTransfer(transfer, owner, "continue")
			switch variant {
			case "structured branch":
				condition.Statement = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{leaf}, nil)
				condition.HasOriginPC = false
				condition.RemoveNext(transfer)
				condition.AddNext(owner)
				condition.EncodedJumps = map[*core.Node]bool{owner: true}
			case "pure return":
				ret.JavaValue = values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
			case "no physical end":
				jump.HasOriginPC = false
			case "wrong end PC":
				jump.OriginPC++
			case "nonjump end":
				jump.Statement = &statements.MiddleStatement{}
			case "ambiguous end":
				jump.AddNext(handler)
			case "foreign header":
				jump.ReplaceNext(header, core.NewNode(&statements.ConditionStatement{}))
			case "missing header origin":
				header.HasOriginPC = false
			case "unknown loop":
				manager.WhileNode = nil
			case "wrong recorded kind":
				manager.loopTransfers[transfer] = loopTransfer{owner, "break"}
			case "wrong recorded target":
				manager.loopTransfers[transfer] = loopTransfer{header, "continue"}
			case "opaque transfer":
				transfer.Statement = &statements.CustomStatement{Name: "continue", LoopTransferKind: "continue"}
			case "changed sealed kind":
				leaf.RetargetSourceTransfer("break", "")
			case "changed label":
				leaf.RetargetSourceTransfer("continue", "OTHER")
			case "changed live target":
				transfer.ReplaceNext(owner, header)
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "multiple handlers":
				other := core.NewNode(&statements.ReturnStatement{})
				other.IsCatchStart = true
				other.CatchHandler = &statements.CatchHandler{EntryPC: 27, ProtectedRanges: [][2]int{{10, 20}}}
				region.AddNext(other)
			case "nonterminal handler":
				handler.Statement = &statements.MiddleStatement{}
			case "protected return operand":
				call.OriginPC = 16
			case "mixed return operand":
				call.Arguments = []values.JavaValue{layerTestCall(16)}
			case "missing operand origin":
				call.HasOriginPC = false
			case "missing return origin":
				ret.HasOriginPC = false
			case "mismatched return PC":
				ret.OriginPC++
			case "return with live successor":
				tail.AddNext(handler)
			case "unmatched encoded edge":
				condition.EncodedJumps = map[*core.Node]bool{tail: true}
			case "unprotected prefix effect":
				prefix := core.NewNode(statements.NewExpressionStatement(layerTestCall(24)))
				region.ReplaceNext(condition, prefix)
				prefix.AddNext(condition)
			case "unprotected predicate":
				condition.Statement.(*statements.ConditionStatement).Condition = layerTestCall(24)
			case "invalid predicate edges":
				condition.AddNext(handler)
			case "opaque prefix":
				condition.Statement = &statements.CustomStatement{}
			case "two normal returns":
				second := core.NewNode(&statements.ReturnStatement{JavaValue: layerTestCall(58), OriginPC: 61, HasOriginPC: true})
				second.OriginPC, second.HasOriginPC = 61, true
				branch := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
				branch.OriginPC, branch.HasOriginPC = 18, true
				condition.ReplaceNext(tail, branch)
				branch.AddNext(tail)
				branch.AddNext(second)
			}
			manager.DominatorMap = GenerateDominatorTree(owner)
			if variant == "stale dominators" {
				manager.DominatorMap = nil
			}
			before := slices.Clone(region.Next)
			got := typedLoopTryNormalBoundary(manager, region, region.Next[0])
			want := variant == "original" || variant == "structured branch" || variant == "pure return"
			if (got == tail) != want {
				t.Fatalf("frontier accepted=%v want=%v", got == tail, want)
			}
			if !slices.Equal(before, region.Next) || region.ProtectedEnd != jump {
				t.Fatal("frontier proof changed original graph")
			}
		})
	}
}

func TestIfContinuationDistinguishesEncodedAndOrdinaryIncomingEdges(t *testing.T) {
	for _, variant := range []string{"continue plus return", "break plus return", "ordinary join", "mixed entries", "only return"} {
		t.Run(variant, func(t *testing.T) {
			ret := core.NewNode(&statements.ReturnStatement{})
			target := core.NewNode(&statements.MiddleStatement{})
			transfer := core.NewNode(statements.NewSourceTransferStatement("continue", ""))
			transfer.IsJmp = true
			transfer.AddNext(target)
			ends, body := []*core.Node{ret, target}, []*core.Node{transfer}
			if variant == "break plus return" {
				transfer.Statement = statements.NewSourceTransferStatement("break", "")
			}
			if variant == "ordinary join" {
				transfer.IsJmp = false
				transfer.Statement = &statements.MiddleStatement{}
			}
			if variant == "mixed entries" {
				normal := core.NewNode(&statements.MiddleStatement{})
				normal.AddNext(target)
				body = append(body, normal)
			}
			if variant == "only return" {
				ends = []*core.Node{ret}
			}
			want := variant == "ordinary join" || variant == "mixed entries"
			if ifHasNormalFallthrough(ends, body) != want {
				t.Fatal("method return confused with encoded loop control")
			}
		})
	}
}
