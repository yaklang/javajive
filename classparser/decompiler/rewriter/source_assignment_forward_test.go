package rewriter

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/javajive/internal/utils"
)

func TestSourceAssignmentForwardDeclarationPasses(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	boolType := types.NewJavaPrimer(types.JavaBoolean)

	t.Run("cross_scope_hoist_preserves_forward_wrapper", func(t *testing.T) {
		id := utils.NewRootVariableId().Next()
		ref := values.NewJavaRef(id, nil, intType)
		ref.VarUid = "ref-10"
		initVal := values.NewJavaLiteral(7, intType)
		as := statements.NewAssignStatement(ref, initVal, true)
		fwd := statements.NewSourceAssignmentForward(as)
		cond := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, boolType)
		cond.VarUid = "ref-1"
		cond.IsParam = true
		ifSt := statements.NewIfStatement(cond, []statements.Statement{fwd}, nil)
		root := []statements.Statement{ifSt, statements.NewReturnStatement(ref)}

		RewriteVar(&root, 2, []*values.JavaRef{cond})

		if len(root) < 3 {
			t.Fatalf("expected hoisted declaration + if + return, got %d root statements", len(root))
		}
		if root[1] != ifSt {
			t.Fatalf("expected if statement at root[1], got %T", root[1])
		}
		kept, ok := ifSt.IfBody[0].(*statements.CustomStatement)
		if !ok || kept != fwd {
			t.Fatalf("expected SourceAssignmentForward wrapper preserved in IfBody[0], got %T", ifSt.IfBody[0])
		}
		if got, fwdOK := kept.SourceAssignmentForward(); !fwdOK || got != as {
			t.Fatalf("expected live forwarded AssignStatement, ok=%v", fwdOK)
		}
		if as.IsFirst || as.IsDeclare || as.JavaValue != initVal {
			t.Fatalf("expected demoted initializer store, got isFirst=%v isDecl=%v val=%v", as.IsFirst, as.IsDeclare, as.JavaValue)
		}
		finalID := as.LeftValue.(*values.JavaRef).Id
		if !topLevelDeclDominatesAllUses(root, finalID) {
			t.Fatal("hoisted forwarded declaration does not dominate all uses")
		}
	})

	t.Run("deduplicate_identity_keeps_wrapper_and_distinct_siblings", func(t *testing.T) {
		id := utils.NewRootVariableId().Next()
		id.SetName("var7")
		ref := values.NewJavaRef(id, nil, intType)
		initVal := values.NewJavaLiteral(3, intType)
		bare := statements.NewDeclareStatement(ref)
		dupStore := statements.NewAssignStatement(ref, initVal, true)
		dupFwd := statements.NewSourceAssignmentForward(dupStore)

		otherID := utils.NewRootVariableId().Next()
		otherID.SetName("var7")
		otherRef := values.NewJavaRef(otherID, nil, intType)
		otherStore := statements.NewAssignStatement(otherRef, initVal, true)
		otherFwd := statements.NewSourceAssignmentForward(otherStore)

		root := []statements.Statement{
			bare,
			dupFwd,
			statements.NewIfStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{otherFwd}, nil),
		}
		dropDuplicateDeclarations(&root)

		kept, ok := root[1].(*statements.CustomStatement)
		if !ok {
			t.Fatalf("expected CustomStatement wrapper preserved at root[1], got %T", root[1])
		}
		got, fwdOK := kept.SourceAssignmentForward()
		if !fwdOK || got == nil || got.IsFirst || got.IsDeclare || got.JavaValue != initVal {
			t.Fatalf("expected demoted forwarded store, got isFirst=%v isDecl=%v", got != nil && got.IsFirst, got != nil && got.IsDeclare)
		}
		if !otherStore.IsFirst {
			t.Fatal("distinct sibling identity was falsely demoted")
		}
	})

	t.Run("restore_explicit_local_decl_on_forwarded_store", func(t *testing.T) {
		id := utils.NewRootVariableId().Next()
		id.SetName("var9")
		ref := values.NewJavaRef(id, nil, intType)
		as := statements.NewAssignStatement(ref, values.NewJavaLiteral(5, intType), false)
		fwd := statements.NewSourceAssignmentForward(as)
		arm := statements.NewIfStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{fwd, statements.NewReturnStatement(ref)}, nil)
		root := []statements.Statement{arm}

		restoreExplicitLocalDeclarations(&root)
		if !as.IsFirst || as.IsDeclare {
			t.Fatalf("expected IsFirst restored on forwarded single def, got isFirst=%v isDecl=%v", as.IsFirst, as.IsDeclare)
		}
		if arm.IfBody[0] != fwd {
			t.Fatal("expected SourceAssignmentForward wrapper preserved")
		}
	})

	t.Run("declaration_insertion_barriers_records_forwarded_decl", func(t *testing.T) {
		id := utils.NewRootVariableId().Next()
		id.SetName("var4")
		ref := values.NewJavaRef(id, nil, intType)
		fwd := statements.NewSourceAssignmentForward(statements.NewAssignStatement(ref, values.NewJavaLiteral(1, intType), true))
		root := []statements.Statement{statements.NewIfStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{fwd}, nil)}
		names, err := DeclarationInsertionBarriers(root, &class_context.ClassContext{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !names["var4"] {
			t.Fatalf("expected var4 barrier from forwarded decl, got %v", names)
		}
	})
}

func TestHoistDeclarationsRewritesInBodyReadsAndUsesIdentity(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	objType := types.NewJavaClass("java.lang.Object")

	mkRef := func(uid, name string, id *utils.VariableId, typ types.JavaType) *values.JavaRef {
		id.SetName(name)
		ref := values.NewJavaRef(id, nil, typ)
		ref.VarUid = uid
		return ref
	}

	t.Run("if_hoist_rewrites_in_body_reads_onto_target_id", func(t *testing.T) {
		idA := utils.NewRootVariableId().Next()
		idB := utils.NewRootVariableId().Next()
		idOut := utils.NewRootVariableId().Next()

		writeA := statements.NewAssignStatement(mkRef("ref-644", "var39", idA, intType), values.NewJavaLiteral(4, intType), true)
		writeB := statements.NewAssignStatement(mkRef("ref-644", "var33", idB, intType), values.NewJavaLiteral(4, intType), true)
		readB := mkRef("ref-644", "var33", idB, intType)
		inBodyUse := statements.NewAssignStatement(mkRef("ref-700", "var57", idOut, intType), values.NewSlotValue(readB, intType), true)

		useA := mkRef("ref-644", "var39", idA, intType)
		afterUse := statements.NewReturnStatement(useA)
		ifSt := statements.NewIfStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{writeA}, []statements.Statement{writeB, inBodyUse})
		root := []statements.Statement{ifSt, afterUse}

		t.Setenv("JDEC_IF_HOIST_OFF", "")
		t.Setenv("JDEC_IF_HOIST_IDENTITY_OFF", "")
		hoistSwitchDeclarations(&root)

		if len(root) != 3 {
			t.Fatalf("expected hoisted decl before if, got len=%d", len(root))
		}
		decl, ok := root[0].(*statements.AssignStatement)
		if !ok || !decl.IsDeclare || decl.LeftValue.(*values.JavaRef).Id != idA {
			t.Fatalf("expected bare decl of idA at root[0], got %T (%v)", root[0], root[0])
		}
		if writeA.LeftValue.(*values.JavaRef).Id != idA || writeA.IsFirst || writeB.LeftValue.(*values.JavaRef).Id != idA || writeB.IsFirst {
			t.Fatalf("expected both branch writes unified onto idA and demoted")
		}
		if inBodyUse.JavaValue.(*values.SlotValue).GetValue().(*values.JavaRef).Id != idA {
			t.Fatalf("in-ElseBody read remained orphaned on idB instead of idA")
		}
	})

	t.Run("if_hoist_rejects_unrelated_name_collision", func(t *testing.T) {
		idA := utils.NewRootVariableId().Next()
		idB := utils.NewRootVariableId().Next()
		idOut := utils.NewRootVariableId().Next()
		idUnrelated := utils.NewRootVariableId().Next()

		writeA := statements.NewAssignStatement(mkRef("ref-644", "var39", idA, intType), values.NewJavaLiteral(4, intType), true)
		writeB := statements.NewAssignStatement(mkRef("ref-644", "var33", idB, intType), values.NewJavaLiteral(4, intType), true)
		readB := mkRef("ref-644", "var33", idB, intType)
		inBodyUse := statements.NewAssignStatement(mkRef("ref-700", "var57", idOut, intType), readB, true)

		unrelatedDecl := statements.NewAssignStatement(mkRef("ref-999", "var39", idUnrelated, objType), values.JavaNull, true)
		unrelatedRet := statements.NewReturnStatement(mkRef("ref-999", "var39", idUnrelated, objType))
		ifSt := statements.NewIfStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{writeA}, []statements.Statement{writeB, inBodyUse})
		root := []statements.Statement{ifSt, unrelatedDecl, unrelatedRet}

		t.Setenv("JDEC_IF_HOIST_OFF", "")
		t.Setenv("JDEC_IF_HOIST_IDENTITY_OFF", "")
		hoistSwitchDeclarations(&root)

		if len(root) != 3 || root[0] != ifSt {
			t.Fatalf("unrelated name collision triggered false if-hoist: len=%d", len(root))
		}
		if !writeB.IsFirst || writeB.LeftValue.(*values.JavaRef).Id != idB || readB.Id != idB {
			t.Fatalf("ElseBody store or read mutated on non-escaping uid")
		}
	})

	t.Run("switch_and_sync_hoist_rewrite_in_body_reads", func(t *testing.T) {
		idA := utils.NewRootVariableId().Next()
		idB := utils.NewRootVariableId().Next()
		idOut := utils.NewRootVariableId().Next()

		writeA := statements.NewAssignStatement(mkRef("ref-500", "var10", idA, intType), values.NewJavaLiteral(1, intType), true)
		writeB := statements.NewAssignStatement(mkRef("ref-500", "var11", idB, intType), values.NewJavaLiteral(2, intType), true)
		readB := mkRef("ref-500", "var11", idB, intType)
		inCaseUse := statements.NewAssignStatement(mkRef("ref-501", "var12", idOut, intType), readB, true)
		sw := &statements.SwitchStatement{
			Value: values.NewJavaLiteral(0, intType),
			Cases: []*statements.CaseItem{
				{Body: []statements.Statement{writeA}},
				{Body: []statements.Statement{writeB, inCaseUse}},
			},
		}
		swRoot := []statements.Statement{sw, statements.NewReturnStatement(mkRef("ref-500", "var11", idB, intType))}
		hoistSwitchDeclarations(&swRoot)
		if readB.Id != idB && writeA.LeftValue.(*values.JavaRef).Id != idB {
			t.Fatalf("switch hoist failed to unify writeA onto idB")
		}
		targetSw := writeA.LeftValue.(*values.JavaRef).Id
		if targetSw == nil || inCaseUse.JavaValue.(*values.JavaRef).Id != targetSw {
			t.Fatalf("switch hoist left in-case read orphaned from targetID")
		}

		// Sync hoister check
		syncIDA := utils.NewRootVariableId().Next()
		syncIDB := utils.NewRootVariableId().Next()
		syncW1 := statements.NewAssignStatement(mkRef("ref-600", "var20", syncIDA, intType), values.NewJavaLiteral(1, intType), true)
		syncW2 := statements.NewAssignStatement(mkRef("ref-600", "var21", syncIDB, intType), values.NewJavaLiteral(2, intType), true)
		syncReadA := mkRef("ref-600", "var20", syncIDA, intType)
		syncInBodyUse := statements.NewAssignStatement(mkRef("ref-601", "var22", utils.NewRootVariableId().Next(), intType), syncReadA, true)
		inner := statements.NewIfStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{syncW1, syncInBodyUse}, []statements.Statement{syncW2})
		syncSt := statements.NewSynchronizedStatement(values.JavaNull, []statements.Statement{inner})
		syncRoot := []statements.Statement{syncSt, statements.NewReturnStatement(mkRef("ref-600", "var21", syncIDB, intType))}
		t.Setenv("JDEC_SYNC_HOIST_OFF", "")
		hoistSwitchDeclarations(&syncRoot)
		if syncReadA.Id != syncIDB || syncW1.LeftValue.(*values.JavaRef).Id != syncIDB {
			t.Fatalf("sync hoist left in-body read or write on stale idA instead of idB")
		}
	})

	t.Run("parallel_arm_hoist_unifies_arm_identities", func(t *testing.T) {
		idA := utils.NewRootVariableId().Next()
		idB := utils.NewRootVariableId().Next()
		idAfter := utils.NewRootVariableId().Next()
		idOutA := utils.NewRootVariableId().Next()
		idOutB := utils.NewRootVariableId().Next()

		bareA := statements.NewDeclareStatement(mkRef("ref-241", "var29", idA, intType))
		innerIfA := statements.NewIfStatement(
			values.NewJavaLiteral(true, boolType),
			[]statements.Statement{statements.NewAssignStatement(mkRef("ref-241", "var29", idA, intType), values.NewJavaLiteral(1, intType), false)},
			[]statements.Statement{statements.NewAssignStatement(mkRef("ref-241", "var29", idA, intType), values.NewJavaLiteral(2, intType), false)},
		)
		readASt := statements.NewAssignStatement(mkRef("ref-300", "var28", idOutA, intType), mkRef("ref-241", "var29", idA, intType), false)

		bareB := statements.NewDeclareStatement(mkRef("ref-246", "var29", idB, intType))
		innerAssignB1 := statements.NewAssignStatement(mkRef("ref-246", "var29", idB, intType), values.NewJavaLiteral(3, intType), false)
		innerAssignB2 := statements.NewAssignStatement(mkRef("ref-246", "var29", idB, intType), values.NewJavaLiteral(4, intType), false)
		innerIfB := statements.NewIfStatement(
			values.NewJavaLiteral(true, boolType),
			[]statements.Statement{innerAssignB1},
			[]statements.Statement{innerAssignB2},
		)
		readBRef := mkRef("ref-246", "var29", idB, intType)
		readBSt := statements.NewAssignStatement(mkRef("ref-301", "var28", idOutB, intType), readBRef, false)

		outerIf := statements.NewIfStatement(
			values.NewJavaLiteral(true, boolType),
			[]statements.Statement{bareA, innerIfA, readASt},
			[]statements.Statement{bareB, innerIfB, readBSt},
		)
		afterSt := statements.NewReturnStatement(mkRef("ref-999", "var29", idAfter, intType))
		root := []statements.Statement{outerIf, afterSt}

		t.Setenv("JDEC_PARALLEL_ARM_HOIST_OFF", "")
		hoistSwitchDeclarations(&root)
		dropEmptySlotAssignments(&root)

		if len(root) != 3 || root[1] != outerIf {
			t.Fatalf("expected [bareDecl, outerIf, afterSt], got len=%d", len(root))
		}
		decl, ok := root[0].(*statements.AssignStatement)
		if !ok || !decl.IsDeclare {
			t.Fatalf("expected bare declaration at root[0], got %T", root[0])
		}
		targetID := decl.LeftValue.(*values.JavaRef).Id
		if targetID != idA {
			t.Fatalf("expected hoisted declaration to carry idA, got %v", targetID)
		}
		if len(outerIf.IfBody) != 2 || len(outerIf.ElseBody) != 2 {
			t.Fatalf("expected demoted bare decls dropped from arms, got IfBody=%d ElseBody=%d", len(outerIf.IfBody), len(outerIf.ElseBody))
		}
		if innerAssignB1.LeftValue.(*values.JavaRef).Id != targetID ||
			innerAssignB2.LeftValue.(*values.JavaRef).Id != targetID ||
			readBRef.Id != targetID {
			t.Fatalf("expected ElseBody assignments and read unified onto targetID (%v)", targetID)
		}

		// Simulate identity-based collision renaming (`resolveLocalNameCollisions`): renaming targetID
		// must rename both arms together so ElseBody never falls back to stale `var29`.
		targetID.SetName("var29_2")
		rendered := outerIf.String(&class_context.ClassContext{})
		if !strings.Contains(rendered, "var29_2") {
			t.Fatalf("expected unified arms to render var29_2, got:\n%s", rendered)
		}
		if utils2.WordMatches(rendered, "var29") {
			t.Fatalf("expected zero stale var29 tokens in unified arms, got:\n%s", rendered)
		}
	})

	t.Run("drop_dangling_caught_exception_discards", func(t *testing.T) {
		excType := types.NewJavaClass("java/lang/Exception")
		newExcVal := func(name string) *values.CustomValue {
			cv := values.NewCustomValue(func(*class_context.ClassContext) string {
				return name
			}, func() types.JavaType {
				return excType
			})
			cv.Flag = "exception"
			return cv
		}

		danglingTop := statements.NewExpressionStatement(newExcVal("Exception;"))
		danglingIf := statements.NewExpressionStatement(newExcVal("Throwable;"))
		danglingElse := statements.NewExpressionStatement(newExcVal("RuntimeException;"))

		savedID := utils.NewRootVariableId().Next()
		keepAssign := statements.NewAssignStatement(mkRef("ref-400", "saved", savedID, excType), newExcVal("caught"), true)
		keepNormal := statements.NewExpressionStatement(values.NewJavaLiteral(1, intType))
		keepThrow := statements.NewThrowStatement(newExcVal("rethrown"))

		ifSt := statements.NewIfStatement(
			values.NewJavaLiteral(true, boolType),
			[]statements.Statement{danglingIf, keepAssign},
			[]statements.Statement{danglingElse, keepNormal, keepThrow},
		)
		root := []statements.Statement{danglingTop, ifSt}

		dropDanglingCaughtExceptionDiscards(&root)

		if len(root) != 1 || root[0] != ifSt {
			t.Fatalf("expected dangling top-level exception discard dropped, got len=%d", len(root))
		}
		if len(ifSt.IfBody) != 1 || ifSt.IfBody[0] != keepAssign {
			t.Fatalf("expected dangling exception in IfBody dropped and AssignStatement kept, got %#v", ifSt.IfBody)
		}
		if len(ifSt.ElseBody) != 2 || ifSt.ElseBody[0] != keepNormal || ifSt.ElseBody[1] != keepThrow {
			t.Fatalf("expected dangling exception in ElseBody dropped while normal expr & throw kept, got %#v", ifSt.ElseBody)
		}

		// Also confirm RewriteVar invokes dangling discard cleanup end-to-end.
		rootViaRewrite := []statements.Statement{
			statements.NewExpressionStatement(newExcVal("Exception;")),
			statements.NewReturnStatement(values.NewJavaLiteral(0, intType)),
		}
		RewriteVar(&rootViaRewrite, 0, nil)
		if len(rootViaRewrite) != 1 {
			t.Fatalf("expected RewriteVar to drop dangling exception discard, got len=%d", len(rootViaRewrite))
		}
	})
}
