package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func cleanupAssertion(t *testing.T, pc int) (*statements.SourceAssertionStatement, *values.FunctionCallExpression, *values.FunctionCallExpression) {
	t.Helper()
	condition := &values.FunctionCallExpression{ClassName: "example.State", FunctionName: "released", Descriptor: "()Z", Kind: values.InvokeStatic, IsStatic: true,
		Object: values.NewJavaClassValue(types.NewJavaClass("example.State")), FuncType: types.NewJavaFuncType("()Z", nil, types.NewJavaPrimer(types.JavaBoolean)), HasOriginPC: true, OriginPC: pc}
	allocation := &values.NewExpression{JavaType: types.NewJavaClass("java.lang.AssertionError"), HasOriginPC: true, OriginPC: pc + 3}
	message := values.NewJavaLiteral("release failed", types.NewJavaPrimer(types.JavaString))
	constructor := &values.FunctionCallExpression{Object: allocation, ClassName: "java.lang.AssertionError", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", Kind: values.InvokeSpecial,
		Arguments: []values.JavaValue{message}, HasOriginPC: true, OriginPC: pc + 6, FuncType: types.NewJavaFuncType("(Ljava/lang/Object;)V", []types.JavaType{types.NewJavaClass("java.lang.Object")}, types.NewJavaPrimer(types.JavaVoid))}
	allocation.ConstructorCall = constructor
	statement, ok := statements.NewSourceAssertionStatement(condition, constructor, pc+9)
	if !ok {
		t.Fatal("invalid authored assertion protocol")
	}
	return statement, condition, constructor
}

func TestFinallyAssertionScopeVisitsConditionAndDeferredMessage(t *testing.T) {
	for _, scenario := range []string{"independent", "condition use", "message use", "stale packet"} {
		t.Run(scenario, func(t *testing.T) {
			st, condition, ctor := cleanupAssertion(t, 100)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			if scenario == "condition use" {
				condition.Arguments = []values.JavaValue{ref}
			}
			if scenario == "message use" {
				ctor.Arguments[0] = ref
				var known bool
				st, known = statements.NewSourceAssertionStatement(condition, ctor, 109)
				if !known {
					t.Fatal("message fixture")
				}
			}
			if scenario == "stale packet" {
				ctor.OriginPC++
			}
			if got := finallyCleanupBindingsScoped([]statements.Statement{st}, nil, map[*utils.VariableId]int{ref.Id: 1}); got != (scenario == "independent") {
				t.Fatalf("scope closed=%v", got)
			}
		})
	}
}

func TestFinallyAssertionComparisonRequiresWholeOriginalFailurePacket(t *testing.T) {
	for _, scenario := range []string{"same protocol", "different condition", "condition origin missing", "protected condition", "protected allocation", "protected constructor", "protected throw", "different message", "mutated argument", "mutated allocation", "mutated invoke", "different statement", "opaque condition", "cyclic condition"} {
		t.Run(scenario, func(t *testing.T) {
			a, ac, _ := cleanupAssertion(t, 100)
			b, bc, ctor := cleanupAssertion(t, 200)
			var other statements.Statement = b
			rows := []core.HandlerRange{{StartPc: 10, EndPc: 20}}
			switch scenario {
			case "different condition":
				bc.FunctionName = "other"
			case "condition origin missing":
				ac.HasOriginPC = false
			case "protected condition", "protected allocation", "protected constructor", "protected throw":
				pc := map[string]uint16{"protected condition": 200, "protected allocation": 203, "protected constructor": 206, "protected throw": 209}[scenario]
				rows = append(rows, core.HandlerRange{StartPc: pc, EndPc: pc + 1})
			case "different message":
				ctor.Arguments[0].(*values.JavaLiteral).Data = "other"
			case "mutated argument":
				ctor.Arguments[0] = values.JavaNull
			case "mutated allocation":
				ctor.Object.(*values.NewExpression).HasOriginPC = false
			case "mutated invoke":
				ctor.OriginPC++
			case "different statement":
				other = statements.NewExpressionStatement(bc)
			case "opaque condition":
				bc.Object = &values.CustomValue{}
			case "cyclic condition":
				bc.Arguments = []values.JavaValue{bc}
			}
			if got := sameFinallyCleanup(rows, a, other, nil, 0); got != (scenario == "same protocol") {
				t.Fatalf("cleanup equivalence=%v", got)
			}
		})
	}
}

func TestFinallyAssertionCleanupRecoversWithoutChangingTheTypedTree(t *testing.T) {
	tr := finallyFixture()
	a, _, _ := cleanupAssertion(t, 100)
	b, _, _ := cleanupAssertion(t, 200)
	ret := tr.TryBody[2]
	tr.TryBody = []statements.Statement{tr.TryBody[0], tr.TryBody[1], a, ret}
	rethrow := tr.CatchBodies[1][1]
	tr.CatchBodies[1] = []statements.Statement{tr.CatchBodies[1][0], b, rethrow}
	view, ok := RecoverCatchAllFinally(tr)
	if !ok || len(view.Cleanup) != 2 || view.Cleanup[1] != b || len(view.TryBody) != 2 || view.TryBody[1] != ret {
		t.Fatal("assertion prevented complete finally recovery")
	}
	if len(tr.TryBody) != 4 || tr.TryBody[2] != a || len(tr.CatchBodies[1]) != 3 || tr.CatchBodies[1][2] != rethrow {
		t.Fatal("rendering proof mutated original statements")
	}
}
