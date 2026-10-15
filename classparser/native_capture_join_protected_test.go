package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Countermodels of source definite assignment. These are not JVM positives;
// the protected-join runtime bank separately checks decoder/handler integration.
func TestNativeCaptureJoinProtectedPredecessorsRequireClosedNormalPaths(t *testing.T) {
	for _, variant := range []string{"capture inside", "capture after", "returning handler", "nested handlers", "renamed", "handler reads local", "handler overwrites local", "handler initializes local", "handler captures local", "handler continues uninitialized", "declaration escapes try", "catch binder aliases local", "nil catch binder", "nil catch ID", "missing handler metadata", "mismatched handlers", "mismatched binders", "catch all finally", "missing protected range", "reversed protected range", "invalid handler PC", "opaque handler", "return before capture", "missing branch definition", "duplicate capture", "work", "memory", "depth", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			blank := statements.NewDeclareStatement(ref)
			one := values.NewJavaLiteral(int64(1), typ)
			write := func() *statements.AssignStatement { return statements.NewAssignStatement(ref, one, false) }
			condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			branch := &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write()}, ElseBody: []statements.Statement{write()}}
			call := &values.FunctionCallExpression{ClassName: "ProtectedProof$1", FunctionName: "<init>", Descriptor: "(J)V", Arguments: []values.JavaValue{ref}, OriginPC: 39, HasOriginPC: true}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("ProtectedProof$1"), ConstructorCall: call, OriginPC: 33, HasOriginPC: true}
			capture := statements.NewReturnStatement(allocation)
			caught := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.RuntimeException"))
			tr := &statements.TryCatchStatement{Exception: []*values.JavaRef{caught}, TryBody: []statements.Statement{branch, capture}, CatchBodies: [][]statements.Statement{{statements.NewThrowStatement(caught)}}, Handlers: []statements.CatchHandler{{EntryPC: 50, ProtectedRanges: [][2]int{{10, 45}}}}}
			body := []statements.Statement{blank, tr}
			var work *workbudget.Budget
			want := variant == "capture inside" || variant == "capture after" || variant == "returning handler" || variant == "nested handlers" || variant == "renamed"
			switch variant {
			case "capture after", "returning handler":
				tr.TryBody = []statements.Statement{branch}
				body = append(body, capture)
				if variant == "returning handler" {
					tr.CatchBodies[0] = []statements.Statement{statements.NewReturnStatement(values.JavaNull)}
				}
			case "nested handlers":
				inner := *tr
				inner.Exception = []*values.JavaRef{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.IllegalArgumentException"))}
				inner.CatchBodies = [][]statements.Statement{{statements.NewThrowStatement(inner.Exception[0])}}
				tr.TryBody = []statements.Statement{&inner}
			case "renamed":
				ref.Id.SetName("chosen")
				caught.Id.SetName("exception")
			case "handler reads local":
				tr.CatchBodies[0] = []statements.Statement{statements.NewReturnStatement(ref)}
			case "handler overwrites local", "handler initializes local":
				tr.CatchBodies[0] = []statements.Statement{write(), statements.NewThrowStatement(caught)}
				if variant == "handler initializes local" {
					tr.TryBody = []statements.Statement{statements.NewThrowStatement(caught)}
					tr.CatchBodies[0] = append(tr.CatchBodies[0][:1], capture)
				}
			case "handler captures local":
				tr.CatchBodies[0] = []statements.Statement{capture}
			case "handler continues uninitialized":
				tr.TryBody = []statements.Statement{branch}
				tr.CatchBodies[0] = nil
				body = append(body, capture)
			case "declaration escapes try":
				tr.TryBody = []statements.Statement{blank, branch}
				body = []statements.Statement{tr, capture}
			case "catch binder aliases local":
				caught.Id = ref.Id
			case "nil catch binder":
				tr.Exception[0] = nil
			case "nil catch ID":
				caught.Id = nil
			case "missing handler metadata":
				tr.Handlers = nil
			case "mismatched handlers":
				tr.Handlers = append(tr.Handlers, tr.Handlers[0])
			case "mismatched binders":
				tr.Exception = append(tr.Exception, caught)
			case "catch all finally":
				tr.Handlers[0].CatchAll = true
			case "missing protected range":
				tr.Handlers[0].ProtectedRanges = nil
			case "reversed protected range":
				tr.Handlers[0].ProtectedRanges[0] = [2]int{45, 10}
			case "invalid handler PC":
				tr.Handlers[0].EntryPC = -1
			case "opaque handler":
				tr.CatchBodies[0] = []statements.Statement{&statements.CustomStatement{ThrownValue: caught}}
			case "return before capture":
				tr.TryBody = []statements.Statement{branch, statements.NewReturnStatement(values.JavaNull), capture}
			case "missing branch definition":
				branch.ElseBody = nil
			case "duplicate capture":
				tr.TryBody = append(tr.TryBody, capture)
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "depth":
				work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			proved, got := nativeCaptureJoinedDeclaration(body, ref, allocation, work)
			if got != want || got && proved != blank {
				t.Fatalf("protected join=%v want=%v declaration identity=%v", got, want, proved == blank)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("bounded refusal must be sticky")
			}
		})
	}
}
