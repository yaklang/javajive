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
