package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestCatchEntryStorePreservesExistingLocalDefinition(t *testing.T) {
	for _, shape := range []string{"existing slot", "wider saved type", "multi-catch", "new catch definition"} {
		t.Run(shape, func(t *testing.T) {
			exact := types.NewJavaClass("probe.FirstFailure")
			var catchType types.JavaType = exact
			savedType := exact
			if shape == "wider saved type" {
				savedType = types.NewJavaClass("java.lang.Throwable")
			}
			if shape == "multi-catch" {
				catchType = types.NewMultiCatchType([]types.JavaType{exact, types.NewJavaClass("probe.SecondFailure")})
				savedType = types.NewJavaClass("java.lang.Exception")
			}
			saved := values.NewJavaRef(utils.NewRootVariableId(), nil, savedType)
			placeholder := values.NewCustomValue(func(*class_context.ClassContext) string { return "Exception" }, func() types.JavaType { return catchType })
			placeholder.Flag = "exception"
			entry := statements.NewAssignStatement(saved, placeholder, shape == "new catch definition")
			entry.OriginPC, entry.HasOriginPC = 23, true
			tail := statements.NewReturnStatement(saved)
			manager := &RewriteManager{}
			caught, body := manager.extractCatchException([]statements.Statement{entry, tail})
			if shape == "new catch definition" {
				if caught != saved || len(body) != 1 || body[0] != tail {
					t.Fatal("fresh catch-local entry store must remain its original definition")
				}
				return
			}
			if caught == saved || values.SameLocal(caught, saved) || caught.Type().String(&class_context.ClassContext{}) != catchType.String(&class_context.ClassContext{}) {
				t.Fatal("handler entry must have distinct identity and original catch type")
			}
			if len(body) != 2 || body[1] != tail {
				t.Fatal("saved handler write or continuation lost")
			}
			copy, ok := body[0].(*statements.AssignStatement)
			if !ok || copy == entry || copy.LeftValue != saved || copy.JavaValue != caught || copy.IsFirst || copy.IsDeclare || copy.OriginPC != entry.OriginPC || copy.HasOriginPC != entry.HasOriginPC {
				t.Fatal("saved slot write changed its destination, origin or definition status")
			}
			if entry.JavaValue != placeholder || entry.LeftValue != saved {
				t.Fatal("extraction mutated its input graph")
			}
		})
	}
}

func TestCatchEntryExtractionLeavesOtherDefinitionsIntact(t *testing.T) {
	typ := types.NewJavaClass("java.lang.Exception")
	manager := &RewriteManager{}
	left := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	right := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	unrelated := statements.NewAssignStatement(left, right, false)
	caught, body := manager.extractCatchException([]statements.Statement{unrelated})
	if len(body) != 1 || body[0] != unrelated || values.SameLocal(caught, left) || values.SameLocal(caught, right) {
		t.Fatal("ordinary first statement is not a synthetic exception entry store")
	}
	placeholder := values.NewCustomValue(func(*class_context.ClassContext) string { return "Exception" }, func() types.JavaType { return typ })
	placeholder.Flag = "exception"
	store := statements.NewAssignStatement(left, placeholder, false)
	second, body := manager.extractCatchException([]statements.Statement{store})
	if values.SameLocal(caught, second) || caught.String(&class_context.ClassContext{}) == second.String(&class_context.ClassContext{}) || len(body) != 1 {
		t.Fatal("sibling catch entry definitions collided")
	}
}

func TestCatchSavedStoreSurvivesBothHandlerClassificationPaths(t *testing.T) {
	for _, marker := range []bool{false, true} {
		typ := types.NewJavaClass("probe.Failure")
		saved := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		placeholder := values.NewCustomValue(func(*class_context.ClassContext) string { return "Exception" }, func() types.JavaType { return typ })
		placeholder.Flag = "exception"
		assignment := statements.NewAssignStatement(saved, placeholder, false)
		assignment.OriginPC, assignment.HasOriginPC = 30, true
		tryBody := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
		catch := core.NewNode(assignment)
		catch.IsCatchStart = marker
		tail := core.NewNode(statements.NewReturnStatement(saved))
		tr := core.NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
		entry := core.NewNode(statements.NewMiddleStatement("start", nil))
		entry.Id, tr.Id, tryBody.Id, catch.Id, tail.Id = 0, 1, 2, 3, 4
		entry.AddNext(tr)
		tr.AddNext(tryBody)
		tr.AddNext(catch)
		tryBody.AddNext(tail)
		catch.AddNext(tail)
		manager := NewRootStatementManager(entry)
		manager.DominatorMap = GenerateDominatorTree(tr)
		if err := TryRewriter(manager, tr); err != nil {
			t.Fatal(err)
		}
		structured, ok := entry.Next[0].Statement.(*statements.TryCatchStatement)
		if !ok || len(structured.Exception) != 1 || len(structured.CatchBodies) != 1 || len(structured.CatchBodies[0]) != 1 {
			t.Fatalf("marker=%t: handler saved store disappeared", marker)
		}
		store, ok := structured.CatchBodies[0][0].(*statements.AssignStatement)
		if !ok || store.LeftValue != saved || store.JavaValue != structured.Exception[0] || values.SameLocal(saved, structured.Exception[0]) || store.OriginPC != 30 {
			t.Fatalf("marker=%t: wrong handler binding", marker)
		}
	}
}
