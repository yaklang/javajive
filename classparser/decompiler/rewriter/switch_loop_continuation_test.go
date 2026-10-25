package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSwitchLatchContinuePreservesBoundaryAndSingleStep(t *testing.T) {
	for _, variant := range []string{"effectful continue", "nested continue", "ordinary tail", "shared switch merge", "foreign predecessor", "unprepared switch", "nonlocal step", "missing local identity", "stack projection", "receiver projection", "nonunit step", "noninteger step", "effectful step", "cyclic tail", "competing exit", "opaque tail", "oversized tail"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
			step := values.NewBinaryExpression(local, values.NewJavaLiteral(1, integer), values.INC, integer)
			root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			arm := core.NewNode(statements.NewExpressionStatement(nil))
			merge := core.NewNode(&statements.ConditionStatement{})
			write := core.NewNode(statements.NewExpressionStatement(nil))
			latch := core.NewNode(step)
			root.AddNext(loop)
			loop.AddNext(owner)
			owner.AddNext(arm)
			owner.AddNext(merge)
			arm.AddNext(latch)
			merge.AddNext(write)
			merge.AddNext(latch)
			write.AddNext(latch)
			latch.AddNext(loop)
			owner.SwitchPrepared, owner.MergeNode = true, merge
			manager := NewRootStatementManager(root)
			manager.currentNodeId = 1000
			manager.WhileNode, manager.SwitchNode = []*core.Node{loop}, []*core.Node{owner}
			switch variant {
			case "nested continue":
				inner := core.NewNode(statements.NewDoWhileStatement(nil, nil))
				loop.ReplaceNextSliceKeepOrder(owner, []*core.Node{inner})
				inner.AddNext(owner)
				manager.WhileNode = append(manager.WhileNode, inner)
			case "ordinary tail":
				owner.RemoveNext(arm)
				merge.RemoveNext(latch)
				merge.AddNext(arm)
			case "shared switch merge":
				owner.MergeNode = latch
			case "foreign predecessor":
				root.AddNext(arm)
			case "unprepared switch":
				owner.SwitchPrepared = false
			case "nonlocal step":
				step.Values[0] = values.NewJavaLiteral(1, integer)
			case "missing local identity":
				local.Id = nil
			case "stack projection":
				local.StackVar = &values.FunctionCallExpression{}
			case "receiver projection":
				local.IsThis = true
			case "nonunit step":
				step.Values[1] = values.NewJavaLiteral(2, integer)
			case "noninteger step":
				local.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "effectful step":
				step.Values[1] = &values.FunctionCallExpression{}
			case "cyclic tail":
				write.AddNext(merge)
			case "competing exit":
				write.AddNext(core.NewNode(&statements.ReturnStatement{}))
			case "opaque tail":
				write.Statement = statements.NewIfStatement(nil, nil, nil)
			case "oversized tail":
				write.RemoveNext(latch)
				cursor := write
				for i := 0; i < 257; i++ {
					next := core.NewNode(statements.NewExpressionStatement(nil))
					cursor.AddNext(next)
					cursor = next
				}
				cursor.AddNext(latch)
			}
			manager.DominatorMap = GenerateDominatorTree(root)
			materializeSwitchLatchContinues(manager, loop)
			want := variant == "effectful continue" || variant == "nested continue"
			if (arm.Next[0] != latch) != want {
				t.Fatalf("continue boundary materialized=%v want=%v", arm.Next[0] != latch, want)
			}
			if !want {
				return
			}
			copy := arm.Next[0]
			increment, ok := copy.Statement.(*values.JavaExpression)
			if !ok || increment == step || increment.Values[0] != local || increment.Values[1] != step.Values[1] || len(copy.Next) != 1 {
				t.Fatal("step identity or once-only execution changed")
			}
			transfer := copy.Next[0]
			kind, label, sealed := transfer.Statement.(*statements.CustomStatement).SourceTransferProtocol()
			if !sealed || kind != "continue" || (label != "") != (variant == "nested continue") || len(transfer.Next) != 1 || transfer.Next[0] != loop || write.Next[0] != latch {
				t.Fatal("normal tail or exact enclosing-loop destination changed")
			}
			materializeSwitchLatchContinues(manager, loop)
			if arm.Next[0] != copy || copy.Next[0] != transfer {
				t.Fatal("second pass duplicated the loop step")
			}
		})
	}
}

func TestSwitchLoopLocalStepRequiresPureExactIntUpdate(t *testing.T) {
	for _, variant := range []string{"add two", "subtract seven", "negative displacement", "min int", "max int", "zero displacement", "other local", "declaration", "first store", "final store", "array store", "division", "call", "floating literal", "wrong literal type", "overflow literal"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
			amount := values.NewJavaLiteral(2, integer)
			expr := values.NewBinaryExpression(local, amount, values.ADD, integer)
			store := statements.NewAssignStatement(local, expr, false)
			want := false
			switch variant {
			case "add two":
				want = true
			case "subtract seven":
				want, expr.Op, amount.Data = true, values.SUB, 7
			case "negative displacement":
				want, amount.Data = true, -2
			case "min int":
				want, amount.Data = true, -2147483648
			case "max int":
				want, amount.Data = true, 2147483647
			case "zero displacement":
				want, amount.Data = true, 0
			case "other local":
				expr.Values[0] = values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
			case "declaration":
				store.IsDeclare = true
			case "first store":
				store.IsFirst = true
			case "final store":
				store.Final = true
			case "array store":
				store.ArrayMember = &values.JavaArrayMember{}
			case "division":
				expr.Op = values.DIV
			case "call":
				expr.Values[1] = &values.FunctionCallExpression{}
			case "floating literal":
				amount.Data = float64(2)
			case "wrong literal type":
				amount.JavaType = types.NewJavaPrimer(types.JavaLong)
			case "overflow literal":
				amount.Data = int64(2147483648)
			}
			copyStep, ok := switchLoopLocalStep(core.NewNode(store))
			if ok != want {
				t.Fatalf("pure local step=%v want=%v", ok, want)
			}
			if !ok {
				return
			}
			first := copyStep().(*statements.AssignStatement)
			second := copyStep().(*statements.AssignStatement)
			value := first.JavaValue.(*values.JavaExpression)
			if first == store || first == second || value == expr || value == second.JavaValue || first.LeftValue != local || value.Values[0] != local || value.Values[1] != amount || value.Op != expr.Op || first.IsDeclare || first.IsFirst {
				t.Fatal("step copy changed binding, displacement, declaration or original expression identity")
			}
		})
	}
}

func TestSwitchAssignmentLatchCopiesOnlyTheSameProtectedEvaluation(t *testing.T) {
	for _, variant := range []string{"call step", "same handler", "different handler", "missing edge origin", "missing store origin", "different store origin", "declaration", "first store", "final store", "array store", "missing binding", "receiver store", "projected store", "hidden transfer", "opaque step", "lambda", "allocation", "dynamic call", "lambda argument", "cyclic slot", "deep call", "wide call", "shared exponential graph"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, integer)
			effect := &values.FunctionCallExpression{FunctionName: "advance", Descriptor: "()I", FuncType: &types.JavaFuncType{ReturnType: integer}}
			store := statements.NewAssignStatement(local, effect, false)
			store.HasOriginPC = true
			store.OriginPC = 80
			root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			arm := core.NewNode(statements.NewExpressionStatement(nil))
			arm.HasOriginPC = true
			arm.OriginPC = 40
			merge := core.NewNode(statements.NewExpressionStatement(nil))
			latch := core.NewNode(store)
			latch.HasOriginPC = true
			latch.OriginPC = 80
			root.AddNext(loop)
			loop.AddNext(owner)
			owner.AddNext(arm)
			owner.AddNext(merge)
			arm.AddNext(latch)
			merge.AddNext(latch)
			latch.AddNext(loop)
			owner.SwitchPrepared = true
			owner.MergeNode = merge
			switch variant {
			case "same handler", "different handler":
				root.HasProtectedRange = true
				root.ProtectedStartPC = 30
				root.ProtectedEndPC = 90
				if variant == "different handler" {
					root.ProtectedEndPC = 60
				}
			case "missing edge origin":
				arm.HasOriginPC = false
			case "missing store origin":
				store.HasOriginPC = false
			case "different store origin":
				store.OriginPC++
			case "declaration":
				store.IsDeclare = true
			case "first store":
				store.IsFirst = true
			case "final store":
				store.Final = true
			case "array store":
				store.ArrayMember = &values.JavaArrayMember{}
			case "missing binding":
				local.Id = nil
			case "receiver store":
				local.IsThis = true
			case "projected store":
				local.StackVar = effect
			case "hidden transfer":
				latch.HideNext = loop
			case "opaque step":
				latch.Statement = statements.NewIfStatement(nil, nil, nil)
			case "lambda":
				store.JavaValue = &values.CustomValue{Flag: "lambda"}
			case "allocation":
				store.JavaValue = &values.NewExpression{}
			case "dynamic call":
				effect.Kind = values.InvokeDynamic
			case "lambda argument":
				effect.Arguments = []values.JavaValue{&values.CustomValue{Flag: "lambda"}}
			case "cyclic slot":
				slot := values.NewSlotValue(effect, nil)
				effect.Object = slot
			case "wide call":
				for i := 0; i < 129; i++ {
					effect.Arguments = append(effect.Arguments, local)
				}
			case "shared exponential graph":
				child := effect
				for i := 0; i < 12; i++ {
					next := &values.FunctionCallExpression{FunctionName: "next", Descriptor: "()I"}
					child.Arguments = []values.JavaValue{next, next}
					child = next
				}
			case "deep call":
				child := effect
				for i := 0; i < 17; i++ {
					next := &values.FunctionCallExpression{FunctionName: "next", Descriptor: "()I"}
					child.Object = next
					child = next
				}
			}
			manager := NewRootStatementManager(root)
			manager.WhileNode = []*core.Node{loop}
			manager.SwitchNode = []*core.Node{owner}
			manager.currentNodeId = 100
			manager.DominatorMap = GenerateDominatorTree(root)
			materializeSwitchLatchContinues(manager, loop)
			want := variant == "call step" || variant == "same handler"
			if (arm.Next[0] != latch) != want {
				t.Fatalf("certified copy=%v want=%v", arm.Next[0] != latch, want)
			}
			if !want {
				return
			}
			copied := arm.Next[0]
			actual := copied.Statement.(*statements.AssignStatement)
			if actual == store || actual.JavaValue != effect || actual.LeftValue != local || actual.OriginPC != 80 || !actual.HasOriginPC || copied.OriginPC != 80 || merge.Next[0] != latch || latch.Statement != store {
				t.Fatal("evaluation, store identity or original normal path changed")
			}
			if len(copied.Next) != 1 || len(copied.Next[0].Next) != 1 || copied.Next[0].Next[0] != loop {
				t.Fatal("copied step does not continue the original loop")
			}
			materializeSwitchLatchContinues(manager, loop)
			if arm.Next[0] != copied {
				t.Fatal("step duplicated twice")
			}
		})
	}
}
