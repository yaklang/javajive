package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func nestedCleanupFixture() (*statements.TryCatchStatement, *statements.TryCatchStatement) {
	tr := finallyFixture()
	callSeed := tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
	throwSeed := tr.CatchBodies[1][1].(*statements.CustomStatement)
	call := func(pc int, name string) *statements.ExpressionStatement {
		x := *callSeed
		x.OriginPC, x.FunctionName = pc, name
		return statements.NewExpressionStatement(&x)
	}
	throw := func(pc int, ref *values.JavaRef) *statements.CustomStatement {
		x := *throwSeed
		x.OriginPC, x.ThrownValue = pc, ref
		return &x
	}
	nested := func(pc int) *statements.TryCatchStatement {
		caught := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
		return &statements.TryCatchStatement{
			TryBody:     []statements.Statement{call(pc, "publish")},
			Exception:   []*values.JavaRef{caught},
			Handlers:    []statements.CatchHandler{{EntryPC: pc + 4, CatchAll: true, ProtectedRanges: [][2]int{{pc, pc + 1}, {pc + 4, pc + 5}}}},
			CatchBodies: [][]statements.Statement{{call(pc+5, "unlock"), throw(pc+6, caught)}},
		}
	}
	normal, cleanup := nested(12), nested(112)
	ret := tr.TryBody[2].(*statements.ReturnStatement)
	ret.OriginPC = 25
	tr.TryBody = []statements.Statement{tr.TryBody[0], normal, call(19, "unlock"), ret}
	tr.Handlers = []statements.CatchHandler{
		{EntryPC: 40, ProtectedRanges: [][2]int{{10, 12}}},
		{EntryPC: 100, CatchAll: true, ProtectedRanges: [][2]int{{10, 12}, {40, 70}}},
	}
	tr.CatchBodies[0][0].(*statements.CustomStatement).OriginPC = 41
	tr.CatchBodies[1] = []statements.Statement{cleanup, call(119, "unlock"), throw(145, tr.Exception[1])}
	return tr, normal
}

func TestFinallyNestedCleanupKeepsSeparateExceptionDomainsAndRethrowBindings(t *testing.T) {
	for _, scenario := range []string{
		"independent catch bindings", "decoded predecessor metadata", "typed handler", "wrong catch type", "unknown catch local", "parameter is not catch binder", "this is not catch binder", "missing rows", "invalid rows", "unprotected publish", "protected handler cleanup", "enclosing-protected handler cleanup", "enclosing-protected handler entry", "protected rethrow", "wrong rethrow", "wrong publish", "wrong unlock", "missing effect origin", "captured catch binding", "source-only entry initializer", "unknown entry metadata", "cycle",
	} {
		t.Run(scenario, func(t *testing.T) {
			tr, normal := nestedCleanupFixture()
			publish := normal.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			unlock := normal.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			rethrow := normal.CatchBodies[0][1].(*statements.CustomStatement)
			switch scenario {
			case "typed handler":
				normal.Handlers[0].CatchAll = false
			case "wrong catch type":
				normal.Exception[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Exception"))
			case "unknown catch local":
				normal.Exception[0] = nil
			case "parameter is not catch binder":
				normal.Exception[0].IsParam = true
			case "this is not catch binder":
				normal.Exception[0].IsThis = true
			case "missing rows":
				normal.Handlers[0].ProtectedRanges = nil
			case "invalid rows":
				normal.Handlers[0].ProtectedRanges[0] = [2]int{12, 12}
			case "unprotected publish":
				publish.OriginPC = 13
			case "protected handler cleanup":
				unlock.OriginPC = 12
			case "enclosing-protected handler cleanup":
				unlock.OriginPC = 11
			case "enclosing-protected handler entry":
				normal.Handlers[0].EntryPC = 11
			case "protected rethrow":
				rethrow.OriginPC = 12
			case "wrong rethrow":
				rethrow.ThrownValue = tr.Exception[1]
			case "wrong publish":
				publish.FunctionName = "other"
			case "wrong unlock":
				unlock.FunctionName = "other"
			case "missing effect origin":
				unlock.HasOriginPC = false
			case "captured catch binding":
				unlock.Arguments, unlock.Descriptor = []values.JavaValue{normal.Exception[0]}, "(Ljava/lang/Throwable;)V"
			case "decoded predecessor metadata":
				normal.EntryInitializers = []*statements.AssignStatement{{LeftValue: normal.Exception[0], JavaValue: values.JavaNull, OriginPC: 11, HasOriginPC: true}}
			case "source-only entry initializer":
				normal.EntryInitializers = []*statements.AssignStatement{{LeftValue: normal.Exception[0], JavaValue: values.JavaNull}}
			case "unknown entry metadata":
				normal.EntryInitializers = []*statements.AssignStatement{nil}
			case "cycle":
				normal.TryBody = []statements.Statement{normal}
			}
			view, known := RecoverCatchAllFinally(tr)
			if known != (scenario == "independent catch bindings" || scenario == "decoded predecessor metadata") {
				t.Fatalf("known=%v", known)
			}
			if known && (len(view.TryBody) != 2 || len(view.CatchBodies) != 1 || len(view.Cleanup) != 2 || view.Cleanup[0] != tr.CatchBodies[1][0] || len(tr.TryBody) != 4 || len(normal.CatchBodies[0]) != 2) {
				t.Fatal("nested region or original cleanup tree changed")
			}
		})
	}
}

func TestFinallyCleanupComparisonBoundsSharedBranchWork(t *testing.T) {
	tr, _ := nestedCleanupFixture()
	var branch statements.Statement = tr.CatchBodies[1][1]
	for i := 0; i < 12; i++ {
		branch = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{branch}, []statements.Statement{branch})
	}
	if sameFinallyCleanup(nil, branch, branch, nil, 0) {
		t.Fatal("a shared branch tree must not restart the comparison work budget")
	}
}

func TestFinallyNestedFoldedArrayNeedsContinuousOriginalProtection(t *testing.T) {
	for _, gap := range []bool{false, true} {
		t.Run(map[bool]string{false: "continuous", true: "unprotected folded store"}[gap], func(t *testing.T) {
			tr, normal := nestedCleanupFixture()
			for _, copy := range []*statements.TryCatchStatement{normal, tr.CatchBodies[1][0].(*statements.TryCatchStatement)} {
				call := copy.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
				pc := call.OriginPC
				integer := types.NewJavaPrimer(types.JavaInteger)
				arrayType := types.NewJavaArrayType(integer)
				array := &values.NewExpression{JavaType: arrayType, OriginPC: pc, HasOriginPC: true, EvaluationEndPC: pc + 2, HasEvaluationEndPC: true,
					Length: []values.JavaValue{values.NewJavaLiteral(1, integer)}, Initializer: []values.JavaValue{values.NewJavaLiteral(7, integer)}}
				call.OriginPC, call.Descriptor, call.Arguments = pc+3, "([I)V", []values.JavaValue{array}
				call.FuncType = types.NewJavaFuncType("([I)V", []types.JavaType{arrayType}, types.NewJavaPrimer(types.JavaVoid))
				copy.Handlers[0].ProtectedRanges[0] = [2]int{pc, pc + 4}
				if gap {
					copy.Handlers[0].ProtectedRanges[0][1] = pc + 1
					copy.Handlers[0].ProtectedRanges = append(copy.Handlers[0].ProtectedRanges, [2]int{pc + 2, pc + 4})
				}
			}
			if _, known := RecoverCatchAllFinally(tr); known == gap {
				t.Fatalf("known=%v gap=%v", known, gap)
			}
		})
	}
}
