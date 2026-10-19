package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func monitorNestedContinuationFixture() (*statements.TryCatchStatement, *statements.TryCatchStatement) {
	outer, branch := monitorContinuationFixture()
	exception := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
	throw := statements.NewThrowStatement(exception)
	throw.HasOriginPC, throw.OriginPC = true, 19
	nested := &statements.TryCatchStatement{Exception: []*values.JavaRef{exception},
		Handlers:    []statements.CatchHandler{{CatchAll: true, EntryPC: 18, ProtectedRanges: [][2]int{{10, 15}}}},
		TryBody:     branch.IfBody,
		CatchBodies: [][]statements.Statement{{throw}}}
	outer.TryBody = []statements.Statement{nested}
	return outer, nested
}

func TestMonitorSingleContinuationKeepsNestedHandlerAndReleaseDomains(t *testing.T) {
	for _, variant := range []string{"original", "normal cleanup outside handler", "handler domain reentry", "mixed handler operand", "missing handler", "missing catch identity", "handler outside monitor", "handler protects released call", "empty interval", "unheld catch path", "unknown release", "foreign release", "release outside monitor", "primitive live-out", "live-out store without origin", "reference live-out", "read before definition", "conditional definition", "catch read before definition", "catch local escapes", "tail effect held", "tail effect unknown", "opaque held effect", "duplicate release", "depth limit", "work limit"} {
		t.Run(variant, func(t *testing.T) {
			outer, nested := monitorNestedContinuationFixture()
			original := nested.TryBody
			switch variant {
			case "normal cleanup outside handler", "handler domain reentry", "mixed handler operand":
				call := *nested.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
				call.OriginPC = 16
				if variant == "mixed handler operand" {
					argument := call
					argument.Descriptor = "()I"
					call.OriginPC = 12
					call.Descriptor, call.Arguments = "(I)V", []values.JavaValue{&argument}
				}
				body := []statements.Statement{nested.TryBody[0], statements.NewExpressionStatement(&call)}
				if variant == "handler domain reentry" {
					body = append(body, nested.TryBody[0])
				}
				nested.TryBody = append(body, nested.TryBody[1:]...)
			case "missing handler":
				nested.Handlers = nil
			case "missing catch identity":
				nested.Exception[0] = nil
			case "handler outside monitor":
				nested.Handlers[0].EntryPC = 21
			case "handler protects released call":
				nested.Handlers[0].ProtectedRanges[0][1] = 22
			case "empty interval":
				nested.Handlers[0].ProtectedRanges[0][1] = 10
			case "unheld catch path":
				nested.CatchBodies[0] = nil
			case "unknown release":
				nested.TryBody[1] = statements.NewMiddleStatement("monitor_exit", nil)
			case "foreign release":
				nested.TryBody[1] = statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 4)
			case "release outside monitor":
				nested.TryBody[1] = statements.NewOriginalMonitorStatement("monitor_exit", nil, 21, 3)
			case "primitive live-out", "live-out store without origin", "reference live-out", "read before definition", "conditional definition", "catch read before definition":
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
				if variant == "reference live-out" {
					ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
				}
				store := statements.NewAssignStatement(ref, values.NewJavaLiteral(7, ref.Type()), true)
				store.HasOriginPC, store.OriginPC = true, 11
				if variant == "live-out store without origin" {
					store.HasOriginPC = false
				}
				nested.TryBody = append([]statements.Statement{store}, nested.TryBody...)
				nested.TryBody[len(nested.TryBody)-1].(*statements.ReturnStatement).JavaValue = ref
				if variant == "read before definition" {
					store.JavaValue = ref
				}
				if variant == "conditional definition" {
					nested.TryBody[0] = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{store}, nil)
				}
				if variant == "catch read before definition" {
					throw := statements.NewThrowStatement(ref)
					throw.HasOriginPC, throw.OriginPC = true, 19
					nested.CatchBodies[0][0] = throw
				}
			case "catch local escapes":
				nested.TryBody[3].(*statements.ReturnStatement).JavaValue = nested.Exception[0]
			case "tail effect held":
				nested.TryBody[2].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 19
			case "tail effect unknown":
				nested.TryBody[2].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "opaque held effect":
				nested.TryBody[0] = statements.NewCustomStatement(nil, nil)
			case "duplicate release":
				nested.TryBody = append(nested.TryBody[:2], append([]statements.Statement{nested.TryBody[1]}, nested.TryBody[2:]...)...)
			case "depth limit":
				for i := 0; i < 40; i++ {
					outer.TryBody = []statements.Statement{statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), outer.TryBody, nil)}
				}
			case "work limit":
				for i := 0; i < 2100; i++ {
					nested.TryBody = append([]statements.Statement{statements.NewMiddleStatement("start", nil)}, nested.TryBody...)
				}
			}
			prefix, held, tail, known := originalMonitorSingleContinuation(outer, 3)
			if known != (variant == "original" || variant == "primitive live-out" || variant == "normal cleanup outside handler") {
				t.Fatalf("single nested frontier=%v", known)
			}
			if known {
				if variant == "normal cleanup outside handler" {
					clone := held[0].(*statements.TryCatchStatement)
					if len(prefix) != 0 || len(held) != 2 || len(clone.TryBody) != 1 || held[1] != nested.TryBody[1] || len(nested.TryBody) != 5 {
						t.Fatal("outside-handler cleanup remained caught, moved across the monitor or mutated the original tree")
					}
					return
				}
				if variant == "primitive live-out" {
					declaration := prefix[0].(*statements.AssignStatement)
					clone := held[0].(*statements.TryCatchStatement)
					store := clone.TryBody[0].(*statements.AssignStatement)
					if len(prefix) != 1 || !declaration.IsDeclare || declaration.IsFirst || declaration.JavaValue != nil || store.IsFirst || store.IsDeclare || !nested.TryBody[0].(*statements.AssignStatement).IsFirst {
						t.Fatal("live-out invented a value, duplicated a store or mutated the input")
					}
					return
				}
				if len(prefix) != 0 {
					t.Fatal("no live-out required a declaration")
				}
				if len(held) != 1 || len(tail) != 2 || tail[0] != original[2] || tail[1] != original[3] {
					t.Fatal("release partition duplicated or changed continuation")
				}
				clone := held[0].(*statements.TryCatchStatement)
				if len(clone.TryBody) != 1 || clone.TryBody[0] != original[0] || clone.CatchBodies[0][0] != nested.CatchBodies[0][0] || len(nested.TryBody) != 4 {
					t.Fatal("original held operation, handler or input tree changed")
				}
			}
		})
	}
}

func TestMonitorNestedHandlerIntervalsRequireCompleteHeldUnion(t *testing.T) {
	for _, variant := range []string{"unsorted adjacent union", "overlapping union", "gap", "negative", "reversed", "past method", "empty", "width", "work"} {
		t.Run(variant, func(t *testing.T) {
			p := monitorSingleContinuationProof{rows: [][2]int{{15, 21}, {10, 15}}, remaining: 100}
			rows := [][2]int{{10, 21}}
			switch variant {
			case "overlapping union":
				p.rows[1][1] = 17
			case "gap":
				p.rows[1][1] = 14
			case "negative":
				rows[0][0] = -1
			case "reversed":
				rows[0] = [2]int{20, 10}
			case "past method":
				rows[0][1] = 65536
			case "empty":
				rows = nil
			case "width":
				rows = make([][2]int, 33)
			case "work":
				p.remaining = 1
			}
			want := variant == "unsorted adjacent union" || variant == "overlapping union"
			if p.rangesHeld(rows) != want {
				t.Fatal("nested protected domain borrowed an unheld interval")
			}
		})
	}
}

func TestMonitorHandlerNormalizationComposesAfterDirectReleaseRecovery(t *testing.T) {
	for _, variant := range []string{"original", "normal return after cleanup", "unknown return", "unknown handler", "handler protects cleanup"} {
		t.Run(variant, func(t *testing.T) {
			outer, nested := monitorNestedContinuationFixture()
			call := *nested.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			call.OriginPC = 16
			cleanup := statements.NewExpressionStatement(&call)
			nested.TryBody = []statements.Statement{nested.TryBody[0], cleanup}
			if variant == "normal return after cleanup" || variant == "unknown return" {
				nested.TryBody = append(nested.TryBody, &statements.ReturnStatement{HasOriginPC: variant != "unknown return", OriginPC: 22})
			}
			originalLength := len(nested.TryBody)
			if variant == "unknown handler" {
				nested.Handlers = nil
			}
			if variant == "handler protects cleanup" {
				nested.Handlers[0].ProtectedRanges = [][2]int{{10, 17}}
			}
			body := normalizeOriginalMonitorHandlers([]statements.Statement{nested}, 3, outer.Handlers[0].ProtectedRanges)
			if variant == "original" || variant == "normal return after cleanup" {
				if len(body) != originalLength || body[1] != cleanup || len(body[0].(*statements.TryCatchStatement).TryBody) != 1 || len(nested.TryBody) != originalLength {
					t.Fatal("normal cleanup remained caught, or original source tree was mutated")
				}
			} else if len(body) != 1 || len(body[0].(*statements.TryCatchStatement).TryBody) != originalLength {
				t.Fatal("unknown or protected cleanup acquired an outside-handler projection")
			}
		})
	}
}
