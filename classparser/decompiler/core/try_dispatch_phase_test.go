package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func loweredDispatchFixture() (*Node, []*Node, []*ExceptionTableEntry) {
	anchor := NewNode(&statements.ConditionStatement{})
	anchor.IsTryCatch, anchor.HasOriginPC, anchor.OriginPC = true, true, 10
	anchor.CatchNodeInfo = []*CatchNode{{StartIndex: 20, EndIndex: 30, ExceptionTypeIndex: 1, OpCode: &OpCode{CurrentOffset: 40}}}
	region := NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
	region.HasProtectedRange, region.ProtectedStartPC, region.ProtectedEndPC = true, 20, 30
	body := NewNode(&statements.ReturnStatement{})
	handler := NewNode(&statements.ReturnStatement{})
	handler.IsCatchStart = true
	handler.CatchHandler = &statements.CatchHandler{EntryPC: 40, ProtectedRanges: [][2]int{{20, 30}}}
	region.AddNext(body)
	region.AddNext(handler)
	anchor.AddNext(region)
	anchor.AddNext(NewNode(&statements.ReturnStatement{}))
	return anchor, []*Node{region}, []*ExceptionTableEntry{{StartPc: 20, EndPc: 30, HandlerPc: 40, CatchType: 1}}
}

func TestTryDispatchCompletionRequiresEveryOriginalRow(t *testing.T) {
	for _, variant := range []string{"complete", "shared typed handler", "catch all", "no provenance", "no rows", "nil row", "nil opcode", "inverted range", "missing original row", "unattached original row", "wrong catch type", "missing region", "opaque region", "missing range", "different region", "missing body", "missing handler", "missing handler evidence", "wrong handler PC", "wrong handler range", "residual handler edge", "partial dispatch", "legacy ordering", "work limit"} {
		t.Run(variant, func(t *testing.T) {
			n, regions, table := loweredDispatchFixture()
			region, ordered := regions[0], true
			handler := region.Next[1]
			switch variant {
			case "shared typed handler":
				extra := *n.CatchNodeInfo[0]
				extra.ExceptionTypeIndex = 2
				n.CatchNodeInfo = append(n.CatchNodeInfo, &extra)
				table = append(table, &ExceptionTableEntry{StartPc: 20, EndPc: 30, HandlerPc: 40, CatchType: 2})
			case "catch all":
				n.CatchNodeInfo[0].ExceptionTypeIndex, table[0].CatchType = 0, 0
				handler.CatchHandler.CatchAll = true
			case "no provenance":
				n.HasOriginPC = false
			case "no rows":
				n.CatchNodeInfo = nil
			case "nil row":
				n.CatchNodeInfo[0] = nil
			case "nil opcode":
				n.CatchNodeInfo[0].OpCode = nil
			case "inverted range":
				n.CatchNodeInfo[0].EndIndex = 10
			case "missing original row":
				table = nil
			case "unattached original row":
				table = append(table, &ExceptionTableEntry{StartPc: 20, EndPc: 30, HandlerPc: 50})
			case "wrong catch type":
				table[0].CatchType = 2
			case "missing region":
				regions = nil
			case "opaque region":
				region.Statement = &statements.CustomStatement{}
			case "missing range":
				region.HasProtectedRange = false
			case "different region":
				region.ProtectedEndPC++
			case "missing body":
				region.RemoveNext(region.Next[0])
			case "missing handler":
				region.RemoveNext(handler)
			case "missing handler evidence":
				handler.CatchHandler = nil
			case "wrong handler PC":
				handler.CatchHandler.EntryPC++
			case "wrong handler range":
				handler.CatchHandler.ProtectedRanges[0][0]++
			case "residual handler edge":
				n.AddNext(handler)
			case "partial dispatch":
				n.CatchNodeInfo = append(n.CatchNodeInfo, &CatchNode{StartIndex: 20, EndIndex: 30, OpCode: &OpCode{CurrentOffset: 50}})
				table = append(table, &ExceptionTableEntry{StartPc: 20, EndPc: 30, HandlerPc: 50})
			case "legacy ordering":
				ordered = false
			case "work limit":
				for len(n.CatchNodeInfo) < 1024 {
					n.CatchNodeInfo = append(n.CatchNodeInfo, n.CatchNodeInfo[0])
					regions = append(regions, region)
				}
			}
			n.recordLoweredTryDispatch(regions, table, ordered)
			want := variant == "complete" || variant == "shared typed handler" || variant == "catch all"
			if got := !n.HasPendingTryDispatch(); got != want {
				t.Fatalf("complete=%v, want %v", got, want)
			}
			if !n.IsTryCatch {
				t.Fatal("historical provenance was erased")
			}
		})
	}
}

func TestTryDispatchCompletionDoesNotAuthorizeStaleOrCopiedAnchors(t *testing.T) {
	for _, variant := range []string{"copy", "changed origin", "changed row", "lost row", "second incomplete lowering"} {
		t.Run(variant, func(t *testing.T) {
			n, regions, table := loweredDispatchFixture()
			n.recordLoweredTryDispatch(regions, table, true)
			if n.HasPendingTryDispatch() {
				t.Fatal("valid lowering was rejected")
			}
			switch variant {
			case "copy":
				copy := *n
				n = &copy
			case "changed origin":
				n.OriginPC++
			case "changed row":
				n.CatchNodeInfo[0].EndIndex++
			case "lost row":
				n.CatchNodeInfo = nil
			case "second incomplete lowering":
				n.recordLoweredTryDispatch(nil, table, true)
			}
			if !n.HasPendingTryDispatch() {
				t.Fatal("stale completion evidence was accepted")
			}
		})
	}
}
