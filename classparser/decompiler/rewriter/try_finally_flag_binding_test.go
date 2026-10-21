package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func finallyFlagFixture() (*statements.TryCatchStatement, *statements.AssignStatement, *values.JavaRef) {
	tr, _, _ := finallyLambdaLetFixture()
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	initial, cleared := values.NewJavaLiteral(true, boolean), values.NewJavaLiteral(false, boolean)
	outer := values.NewJavaRef(utils.NewRootVariableId(), initial, boolean.Copy())
	outer.MarkOriginalLocalDeclaration(5, 2, initial)
	inner := values.NewJavaRef(utils.NewRootVariableId(), cleared, boolean.Copy())
	inner.MarkOriginalLocalDeclaration(11, 2, cleared)
	store := statements.NewAssignStatement(inner, cleared, true)
	store.OriginPC, store.HasOriginPC = 11, true
	store.MarkOriginalLocalStore(11, 2)
	normal := statements.NewIfStatement(inner, tr.TryBody[1:4], nil)
	exceptional := statements.NewIfStatement(outer, tr.CatchBodies[1][:3], nil)
	tr.TryBody = []statements.Statement{tr.TryBody[0], store, normal, tr.TryBody[4]}
	tr.Exception = tr.Exception[1:]
	tr.Handlers = tr.Handlers[1:]
	tr.Handlers[0].ProtectedRanges = [][2]int{{10, 12}}
	tr.CatchBodies = [][]statements.Statement{{exceptional, tr.CatchBodies[1][3]}}
	return tr, store, outer
}

func TestFinallyFlagSourceBindingRequiresOriginalSlotAndClosedUses(t *testing.T) {
	for _, variant := range []string{"same", "renamed", "compare zero", "reverse compare one", "negated", "opposite polarity", "wrong slot", "missing store", "forged declaration", "unprotected store", "initial definition inside region", "not boolean", "different guard", "handler else", "normal else", "prefix use", "normal capture", "handler capture", "continuation use", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			tr, store, outer := finallyFlagFixture()
			inner := store.LeftValue.(*values.JavaRef)
			switch variant {
			case "compare zero":
				tr.TryBody[2].(*statements.IfStatement).Condition = &values.JavaExpression{Op: values.NEQ, Values: []values.JavaValue{inner, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
			case "reverse compare one":
				tr.CatchBodies[0][0].(*statements.IfStatement).Condition = &values.JavaExpression{Op: values.EQ, Values: []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), outer}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
			case "negated", "opposite polarity":
				tr.TryBody[2].(*statements.IfStatement).Condition = &values.JavaExpression{Op: values.Not, Values: []values.JavaValue{inner}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
				if variant == "negated" {
					tr.CatchBodies[0][0].(*statements.IfStatement).Condition = &values.JavaExpression{Op: values.Not, Values: []values.JavaValue{outer}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
				}
			case "renamed":
				inner.Id.SetName("normalFlag")
				outer.Id.SetName("handlerFlag")
			case "wrong slot", "forged declaration":
				ref := values.NewJavaRef(utils.NewRootVariableId(), store.JavaValue, inner.Type())
				if variant == "wrong slot" {
					ref.MarkOriginalLocalDeclaration(11, 3, store.JavaValue)
				}
				store.LeftValue = ref
				tr.TryBody[2].(*statements.IfStatement).Condition = ref
			case "missing store":
				store.HasOriginPC = false
			case "unprotected store":
				tr.Handlers[0].ProtectedRanges[0][1] = 11
			case "initial definition inside region":
				tr.Handlers[0].ProtectedRanges[0][0] = 5
			case "not boolean":
				inner.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "different guard":
				tr.TryBody[2].(*statements.IfStatement).Condition = values.NewJavaRef(utils.NewRootVariableId(), nil, inner.Type())
			case "handler else":
				tr.CatchBodies[0][0].(*statements.IfStatement).ElseBody = []statements.Statement{tr.TryBody[0]}
			case "normal else":
				tr.TryBody[2].(*statements.IfStatement).ElseBody = []statements.Statement{tr.TryBody[0]}
			case "prefix use":
				tr.TryBody = append([]statements.Statement{statements.NewExpressionStatement(outer)}, tr.TryBody...)
			case "normal capture":
				x := tr.TryBody[2].(*statements.IfStatement)
				x.IfBody = append([]statements.Statement{statements.NewExpressionStatement(inner)}, x.IfBody...)
			case "handler capture":
				x := tr.CatchBodies[0][0].(*statements.IfStatement)
				x.IfBody = append([]statements.Statement{statements.NewExpressionStatement(outer)}, x.IfBody...)
			case "continuation use":
				tr.TryBody[3].(*statements.ReturnStatement).JavaValue = outer
			case "cycle":
				tr.TryBody = append([]statements.Statement{tr}, tr.TryBody...)
			}
			view, got := finallyFlagSourceView(tr)
			want := variant == "same" || variant == "renamed" || variant == "compare zero" || variant == "reverse compare one" || variant == "negated"
			if got != want {
				t.Fatalf("flag source binding=%v; want %v", got, want)
			}
			if got {
				originalRef, _, _ := finallyFlagCondition(tr.TryBody[2].(*statements.IfStatement).Condition, 0)
				if store.LeftValue != inner || !store.IsFirst || view.TryBody[1].(*statements.AssignStatement).LeftValue != outer || view.TryBody[1].(*statements.AssignStatement).IsFirst || originalRef != inner {
					t.Fatal("source binding must leave the canonical declarations and original stores unchanged")
				}
				if region, complete := RecoverCatchAllFinallyForSource(tr); !complete || len(region.Cleanup) != 1 || len(region.Exceptions) != 0 || len(region.TryBody) != 3 {
					t.Fatal("one guarded lambda cleanup must survive once as finally")
				}
			}
		})
	}
}
