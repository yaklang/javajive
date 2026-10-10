package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/classparser/decompiler/rewriter"
	"testing"
)

// A source finally implicitly propagates the protected operation's failure.
// Only the unchanged synthetic rethrow disappears; explicit checked payloads
// in the body, typed handlers and terminal cleanup must remain visible.
func TestCheckedEscapeFinallyViewRetainsExplicitThrows(t *testing.T) {
	fixture := func() *statements.TryCatchStatement {
		ref := func(name string) *values.JavaRef {
			return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
		}
		caught, primary := ref("java.io.IOException"), ref("java.lang.Throwable")
		call := func(pc int, name string) statements.Statement {
			return statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "fixture/Effects", FunctionName: name, Descriptor: "()V", IsStatic: true, Kind: values.InvokeStatic, Object: values.NewJavaClassValue(types.NewJavaClass("fixture.Effects")), OriginPC: pc, HasOriginPC: true, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))})
		}
		ret := statements.NewReturnStatement(nil)
		ret.OriginPC, ret.HasOriginPC = 15, true
		tr := statements.NewTryCatchStatement([]statements.Statement{call(10, "body"), call(12, "cleanup"), ret}, [][]statements.Statement{{&statements.CustomStatement{ThrownValue: caught, OriginPC: 21, HasOriginPC: true}}, {call(42, "cleanup"), &statements.CustomStatement{ThrownValue: primary, OriginPC: 45, HasOriginPC: true}}})
		tr.Exception = []*values.JavaRef{caught, primary}
		tr.Handlers = []statements.CatchHandler{{EntryPC: 20, ProtectedRanges: [][2]int{{10, 12}}}, {EntryPC: 40, CatchAll: true, ProtectedRanges: [][2]int{{10, 12}, {20, 40}}}}
		return tr
	}
	for _, variant := range []string{"proved finally", "typed catch is not finally", "missing coverage", "protected normal cleanup", "changed payload", "missing rethrow origin", "body checked throw", "cleanup checked throw"} {
		t.Run(variant, func(t *testing.T) {
			tr := fixture()
			switch variant {
			case "typed catch is not finally":
				tr.Handlers[1].CatchAll = false
			case "missing coverage":
				tr.Handlers[1].ProtectedRanges = nil
			case "protected normal cleanup":
				tr.Handlers[1].ProtectedRanges[0][1] = 13
			case "changed payload":
				tr.CatchBodies[1][1].(*statements.CustomStatement).ThrownValue = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.io.IOException"))
			case "missing rethrow origin":
				tr.CatchBodies[1][1].(*statements.CustomStatement).HasOriginPC = false
			case "body checked throw":
				tr.TryBody = []statements.Statement{&statements.CustomStatement{ThrownValue: tr.Exception[0], OriginPC: 10, HasOriginPC: true}, tr.TryBody[1], tr.TryBody[2]}
			case "cleanup checked throw":
				flag := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
				failure := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.io.IOException"))
				tr.TryBody[1] = statements.NewIfStatement(flag, []statements.Statement{&statements.CustomStatement{ThrownValue: failure, OriginPC: 13, HasOriginPC: true}}, nil)
				tr.CatchBodies[1] = []statements.Statement{tr.CatchBodies[1][0], statements.NewIfStatement(flag, []statements.Statement{&statements.CustomStatement{ThrownValue: failure, OriginPC: 44, HasOriginPC: true}}, []statements.Statement{tr.CatchBodies[1][1]})}
				tr.TryBody = append(tr.TryBody[:1:1], tr.CatchBodies[1][0], tr.TryBody[1], tr.TryBody[2])
				// The copied cleanup call also needs its distinct original use PC.
				tr.TryBody[1] = statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "fixture/Effects", FunctionName: "cleanup", Descriptor: "()V", IsStatic: true, Kind: values.InvokeStatic, Object: values.NewJavaClassValue(types.NewJavaClass("fixture.Effects")), OriginPC: 12, HasOriginPC: true, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))})
			}
			_, recovered := rewriter.RecoverCatchAllFinally(tr)
			wantView := variant == "proved finally" || variant == "cleanup checked throw"
			if recovered != wantView {
				t.Fatalf("finally proof=%v want%v", recovered, wantView)
			}
			got := checkedEscapeThrownTypes([]statements.Statement{tr}, func(string) (callbinding.Class, bool) { return callbinding.Class{}, false })
			if got[21] != "java/io/IOException" {
				t.Fatalf("typed explicit payload lost: %v", got)
			}
			if wantView && got[45] != "" {
				t.Fatalf("removed synthetic rethrow counted: %v", got)
			}
			if !wantView && variant != "missing rethrow origin" && got[45] == "" {
				t.Fatalf("unproved rethrow discarded: %v", got)
			}
			if variant == "body checked throw" && got[10] != "java/io/IOException" {
				t.Fatalf("body payload lost: %v", got)
			}
			if variant == "cleanup checked throw" && got[44] != "java/io/IOException" {
				t.Fatalf("cleanup payload lost: %v", got)
			}
			if len(tr.CatchBodies) != 2 || len(tr.Exception) != 2 {
				t.Fatal("source analysis mutated original ownership")
			}
		})
	}
}
