package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyFallthroughRequiresCompleteCleanupAndOutsideSuffix(t *testing.T) {
	for _, change := range []string{
		"original", "outside nested handler", "missing entry state", "wrong normal call", "wrong normal receiver",
		"protected normal cleanup", "unknown normal cleanup", "wrong rethrow identity", "primary overwritten",
		"protected tail call", "mixed tail operand", "unknown tail operand", "nested protected handler", "nested unknown handler",
		"missing handler rows", "invalid handler rows", "different handler arity", "opaque tail", "cyclic tail", "no tail", "budget",
	} {
		t.Run(change, func(t *testing.T) {
			tr := primaryResourceFixture()
			normal := tr.TryBody[0]
			work := statements.NewExpressionStatement(layerTestCall(10))
			tailCall := layerTestCall(100)
			tail := statements.NewExpressionStatement(tailCall)
			tr.TryBody = []statements.Statement{work, normal, tail}
			switch change {
			case "outside nested handler", "nested protected handler", "nested unknown handler":
				caught := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
				ret := statements.NewReturnStatement(values.JavaNull)
				ret.HasOriginPC, ret.OriginPC = true, 120
				nested := statements.NewTryCatchStatement([]statements.Statement{tail}, [][]statements.Statement{{ret}})
				nested.Exception = []*values.JavaRef{caught}
				nested.Handlers = []statements.CatchHandler{{EntryPC: 110, ProtectedRanges: [][2]int{{99, 105}}}}
				if change == "nested protected handler" {
					nested.Handlers[0].ProtectedRanges = [][2]int{{2, 20}, {99, 105}}
				} else if change == "nested unknown handler" {
					nested.Handlers[0].ProtectedRanges = nil
				}
				tr.TryBody[2] = nested
			case "missing entry state":
				tr.EntryInitializers = nil
			case "wrong normal call":
				normal.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
			case "wrong normal receiver":
				normal.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Resource"))
			case "protected normal cleanup":
				normal.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 19
			case "unknown normal cleanup":
				normal.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "wrong rethrow identity":
				tr.CatchBodies[1][1].(*statements.CustomStatement).ThrownValue = tr.Exception[0]
			case "primary overwritten":
				as := statements.NewAssignStatement(tr.EntryInitializers[0].LeftValue, values.JavaNull, false)
				as.HasOriginPC, as.OriginPC = true, 5
				tr.TryBody = append([]statements.Statement{as}, tr.TryBody...)
			case "protected tail call":
				tailCall.OriginPC = 10
			case "mixed tail operand":
				tailCall.Arguments = []values.JavaValue{layerTestCall(10)}
			case "unknown tail operand":
				tailCall.HasOriginPC = false
			case "missing handler rows":
				tr.Handlers[0].ProtectedRanges = nil
			case "invalid handler rows":
				tr.Handlers[0].ProtectedRanges = [][2]int{{20, 2}}
			case "different handler arity":
				tr.Handlers = tr.Handlers[:1]
			case "opaque tail":
				tail.Expression = &values.CustomValue{}
			case "cyclic tail":
				tailCall.Arguments = []values.JavaValue{tailCall}
			case "no tail":
				tr.TryBody = tr.TryBody[:2]
			case "budget":
				for i := 0; i < 129; i++ {
					tr.TryBody = append(tr.TryBody, statements.NewExpressionStatement(layerTestCall(100)))
				}
			}
			before := slices.Clone(tr.TryBody)
			suffix := factorFinallyFallthroughTail(tr)
			want := change == "original" || change == "outside nested handler"
			if (len(suffix) != 0) != want {
				t.Fatalf("factored=%v want=%v", suffix, want)
			}
			if !want && !slices.Equal(before, tr.TryBody) {
				t.Fatal("failed proof changed original source statements")
			}
			if want {
				if len(tr.TryBody) != 2 || len(suffix) != 1 || suffix[0] != before[2] || tr.TryBody[1] != normal {
					t.Fatal("normal cleanup or original continuation was copied, reordered or lost")
				}
				if _, known := RecoverCatchAllFinallyForSource(tr); !known {
					t.Fatal("factored prefix lost its complete finally proof")
				}
			}
		})
	}
}

func TestFactoredFinallyBoundaryRequiresExactConsumedStatementAndDomain(t *testing.T) {
	for _, change := range []string{"original", "jump boundary", "copied statement", "other statement", "missing consumed", "ambiguous consumed", "different boundary", "protected call", "mixed call", "unknown call", "cyclic call", "catch all", "unknown handler", "missing interval"} {
		t.Run(change, func(t *testing.T) {
			call := layerTestCall(20)
			statement := statements.NewExpressionStatement(call)
			original := core.NewNode(statement)
			original.HasOriginPC, original.OriginPC = true, 20
			replacement := core.NewNode(statement)
			consumed := []*core.Node{original}
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange, region.ProtectedStartPC, region.ProtectedEndPC = true, 0, 10
			region.ProtectedEnd = original
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(handler)
			switch change {
			case "jump boundary":
				jump := core.NewNode(&statements.GOTOStatement{})
				jump.AddNext(original)
				region.ProtectedEnd = jump
			case "copied statement":
				copy := *statement
				replacement.Statement = &copy
			case "other statement":
				replacement.Statement = statements.NewExpressionStatement(layerTestCall(20))
			case "missing consumed":
				consumed = nil
			case "ambiguous consumed":
				consumed = append(consumed, core.NewNode(statement))
			case "different boundary":
				region.ProtectedEnd = core.NewNode(statement)
			case "protected call":
				call.OriginPC = 7
			case "mixed call":
				call.Arguments = []values.JavaValue{layerTestCall(7)}
			case "unknown call":
				call.HasOriginPC = false
			case "cyclic call":
				call.Arguments = []values.JavaValue{call}
			case "catch all":
				handler.CatchHandler.CatchAll = true
			case "unknown handler":
				handler.CatchHandler = nil
			case "missing interval":
				region.HasProtectedRange = false
			}
			before := region.ProtectedEnd
			retargetFactoredStatementBoundary([]*core.Node{region}, consumed, replacement)
			want := change == "original" || change == "jump boundary"
			if (region.ProtectedEnd == replacement) != want || !want && region.ProtectedEnd != before {
				t.Fatalf("retarget=%t want=%t", region.ProtectedEnd == replacement, want)
			}
			if want && (!replacement.HasOriginPC || replacement.OriginPC != original.OriginPC) {
				t.Fatal("lost original source consumer witness")
			}
		})
	}
}
