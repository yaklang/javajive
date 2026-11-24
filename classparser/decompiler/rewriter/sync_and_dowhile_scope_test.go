package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestRewriteVarSynchronizedSiblingLoopsAndEscapingLocal(t *testing.T) {
	strTyp := types.NewJavaClass("java.lang.String")
	boolTyp := types.NewJavaPrimer(types.JavaBoolean)
	ctx := &class_context.ClassContext{PackageName: "pkg", ClassName: "pkg.Test"}

	buildSyncAST := func() ([]statements.Statement, *statements.SynchronizedStatement, *values.JavaRef, *values.JavaRef) {
		lock := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
		lock.IsParam = true
		cond1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolTyp)
		cond1.IsParam = true
		cond2 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolTyp)
		cond2.IsParam = true
		condFlag := values.NewJavaRef(utils.NewRootVariableId(), nil, boolTyp)
		condFlag.IsParam = true

		// Two distinct local identities sharing pre-rename placeholder "var9" in sibling loops
		// inside synchronized(lock): loop1 declares String var9, loop2 assigns & reads var9
		// across nested branches without IsFirst set.
		oldLoop1ID := utils.NewRootVariableId()
		oldLoop1ID.SetName("var9")
		loop1Def := values.NewJavaRef(oldLoop1ID, nil, strTyp)

		oldLoop2ID := utils.NewRootVariableId()
		oldLoop2ID.SetName("var9")
		loop2AssignA := values.NewJavaRef(oldLoop2ID, nil, strTyp)
		loop2AssignA.VarUid = "slot-shared-uid"
		loop2AssignB := values.NewJavaRef(oldLoop2ID, nil, strTyp)
		loop2AssignB.VarUid = "slot-shared-uid"
		loop2Read := values.NewJavaRef(oldLoop2ID, nil, strTyp)
		loop2Read.VarUid = "slot-shared-uid"

		// Escaping local assigned inside synchronized and read afterwards.
		escapingID := utils.NewRootVariableId()
		escapingID.SetName("var12")
		escapingWrite := values.NewJavaRef(escapingID, nil, strTyp)
		escapingWrite.VarUid = "slot-escape-uid"
		escapingRead := values.NewJavaRef(escapingID, nil, strTyp)
		escapingRead.VarUid = "slot-escape-uid"

		loop1 := &statements.WhileStatement{
			ConditionValue: cond1,
			Body: []statements.Statement{
				&statements.IfStatement{
					Condition: condFlag,
					IfBody: []statements.Statement{
						statements.NewAssignStatement(loop1Def, values.NewJavaLiteral(`"a"`, strTyp), true),
						statements.NewExpressionStatement(loop1Def),
					},
				},
			},
		}
		loop2 := &statements.WhileStatement{
			ConditionValue: cond2,
			Body: []statements.Statement{
				&statements.IfStatement{
					Condition: condFlag,
					IfBody: []statements.Statement{
						&statements.IfStatement{
							Condition: cond1,
							IfBody:    []statements.Statement{statements.NewAssignStatement(loop2AssignA, values.NewJavaLiteral(`""`, strTyp), false)},
							ElseBody:  []statements.Statement{statements.NewAssignStatement(loop2AssignB, values.NewJavaLiteral(`"x"`, strTyp), false)},
						},
						statements.NewAssignStatement(escapingWrite, loop2Read, false),
					},
				},
			},
		}

		syncStmt := statements.NewSynchronizedStatement(lock, []statements.Statement{loop1, loop2})
		root := []statements.Statement{syncStmt, statements.NewReturnStatement(escapingRead)}
		return root, syncStmt, lock, cond1
	}

	root, syncStmt, lock, cond1 := buildSyncAST()
	RewriteVar(&root, 1, []*values.JavaRef{lock, cond1}, ctx)

	// Every generated local inside synchronized (including loop2's if/else local and the escaping return)
	// must be cleanly covered in dominating scope.
	if len(root) < 2 {
		t.Fatalf("expected escaping local declared before synchronized, got root len=%d", len(root))
	}
	ret := root[len(root)-1].(*statements.ReturnStatement)
	retRef := ret.JavaValue.(*values.JavaRef)
	if !isDeclaredAtTopLevel(root, retRef.Id) {
		t.Fatalf("escaping local %s was not declared at method scope before synchronized", retRef.Id.String())
	}

	// Verify loop2's internal local has its own declaration dominating loop2Read.
	loop2 := syncStmt.Body[len(syncStmt.Body)-1].(*statements.WhileStatement)
	loop2If := loop2.Body[0].(*statements.IfStatement)
	assignToEscape := loop2If.IfBody[len(loop2If.IfBody)-1].(*statements.AssignStatement)
	readRef := assignToEscape.JavaValue.(*values.JavaRef)
	if !topLevelDeclDominatesAllUses(loop2If.IfBody, readRef.Id) {
		t.Fatalf("loop2 local %s is not declared in dominating scope inside synchronized:\n%s",
			readRef.Id.String(), syncStmt.String(ctx))
	}

	// Verify JDEC_SYNC_REWRITE_OFF=1 restores legacy behavior (does not rewrite loop2 inside synchronized).
	t.Setenv("JDEC_SYNC_REWRITE_OFF", "1")
	rootOff, syncOff, lockOff, condOff := buildSyncAST()
	RewriteVar(&rootOff, 1, []*values.JavaRef{lockOff, condOff}, ctx)
	loop2Off := syncOff.Body[len(syncOff.Body)-1].(*statements.WhileStatement)
	loop2IfOff := loop2Off.Body[0].(*statements.IfStatement)
	readOff := loop2IfOff.IfBody[len(loop2IfOff.IfBody)-1].(*statements.AssignStatement).JavaValue.(*values.JavaRef)
	if topLevelDeclDominatesAllUses(loop2IfOff.IfBody, readOff.Id) {
		t.Fatalf("JDEC_SYNC_REWRITE_OFF=1 should preserve un-rewritten synchronized block")
	}
}
