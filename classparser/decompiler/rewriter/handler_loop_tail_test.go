package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestHandlerSharedStepPreservesExceptionalContinueOnly(t *testing.T) {
	for _, variant := range []string{"unit step", "nonunit step", "decrement", "missing origin", "missing transfer record", "changed target", "changed sealed kind", "opaque continue", "wrong loop", "foreign handler", "foreign step", "extra step successor", "throwing step", "nonlocal step", "unknown handler", "wrong range", "hidden edge", "already encoded", "unqualified inner loop", "qualified outer loop"} {
		t.Run(variant, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{})
			loop := core.NewNode(statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil))
			header := core.NewNode(&statements.ConditionStatement{})
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange, region.ProtectedStartPC, region.ProtectedEndPC = true, 10, 20
			handler := core.NewNode(&statements.AssignStatement{})
			handler.IsCatchStart, handler.HasOriginPC, handler.OriginPC = true, true, 30
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 30, ProtectedRanges: [][2]int{{10, 20}}}
			normal, prefix, continuation := core.NewNode(&statements.AssignStatement{}), core.NewNode(&statements.ExpressionStatement{}), core.NewNode(&statements.ExpressionStatement{})
			typ := types.NewJavaPrimer(types.JavaInteger)
			local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			expr := values.NewBinaryExpression(local, values.NewJavaLiteral(1, typ), values.INC, typ)
			step := core.NewNode(expr)
			step.HasOriginPC, step.OriginPC = true, 50
			leaf := statements.NewSourceTransferStatement("continue", "")
			transfer := core.NewNode(leaf)
			transfer.IsJmp = true
			exit := core.NewNode(statements.NewReturnStatement(nil))
			root.AddNext(loop)
			loop.AddNext(header)
			header.AddNext(region)
			header.AddNext(exit)
			region.AddNext(normal)
			region.AddNext(handler)
			normal.AddNext(continuation)
			continuation.AddNext(step)
			handler.AddNext(prefix)
			prefix.AddNext(step)
			step.AddNext(transfer)
			transfer.AddNext(loop)
			m := NewRootStatementManager(root)
			m.WhileNode = []*core.Node{loop}
			m.recordLoopTransfer(transfer, loop, "continue", step)
			want := variant == "unit step" || variant == "nonunit step" || variant == "decrement" || variant == "qualified outer loop"
			switch variant {
			case "nonunit step":
				expr = values.NewBinaryExpression(local, values.NewJavaLiteral(-37, typ), values.ADD, typ)
				step.Statement = statements.NewAssignStatement(local, expr, false)
			case "decrement":
				expr.Op = values.DEC
			case "missing origin":
				step.HasOriginPC = false
			case "missing transfer record":
				delete(m.loopTransfers, transfer)
			case "changed target":
				transfer.ReplaceNext(loop, header)
			case "changed sealed kind":
				leaf.RetargetSourceTransfer("break", "")
			case "opaque continue":
				transfer.Statement = &statements.CustomStatement{Name: "continue", LoopTransferKind: "continue"}
			case "wrong loop":
				m.WhileNode = nil
			case "foreign handler":
				root.AddNext(handler)
			case "foreign step":
				root.AddNext(step)
			case "extra step successor":
				step.AddNext(exit)
			case "throwing step":
				step.Statement = statements.NewExpressionStatement(layerTestCall(50))
			case "nonlocal step":
				local.CustomValue = values.NewCustomValue(nil, nil)
			case "unknown handler":
				handler.CatchHandler = nil
			case "wrong range":
				handler.CatchHandler.ProtectedRanges = [][2]int{{0, 9}}
			case "hidden edge":
				prefix.HideNext = step
			case "already encoded":
				prefix.EncodedJumps = map[*core.Node]bool{step: true}
			case "unqualified inner loop", "qualified outer loop":
				inner := core.NewNode(statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil))
				header.ReplaceNext(region, inner)
				inner.AddNext(region)
				m.WhileNode = append(m.WhileNode, inner)
				if variant == "qualified outer loop" {
					loop.Statement.(*statements.DoWhileStatement).Label = "outer_scope"
					leaf.RetargetSourceTransfer("continue", "outer_scope")
				}
			}
			m.DominatorMap = GenerateDominatorTree(root)
			materializeHandlerLoopTails(m, region)
			if (prefix.Next[0] != step) != want {
				t.Fatalf("private step=%v want=%v", prefix.Next[0] != step, want)
			}
			if continuation.Next[0] != step || step.Next[0] != transfer || normal.Next[0] != continuation || region.Next[0] != normal || region.Next[1] != handler {
				t.Fatal("changed normal continuation or exceptional entry")
			}
			if want {
				copy := prefix.Next[0]
				jump := copy.Next[0]
				if !copy.HasOriginPC || copy.OriginPC != step.OriginPC || len(copy.Next) != 1 || len(jump.Next) != 1 || jump.Next[0] != loop || m.loopTransfers[jump].target != loop {
					t.Fatal("private tail lost original step or loop target")
				}
				kind, label, sealed := jump.Statement.(*statements.CustomStatement).SourceTransferProtocol()
				if !sealed || kind != "continue" || label != loop.Statement.(*statements.DoWhileStatement).Label {
					t.Fatal("private transfer changed the original target label")
				}
				var copied *values.JavaExpression
				switch st := copy.Statement.(type) {
				case *values.JavaExpression:
					copied = st
				case *statements.AssignStatement:
					copied, _ = st.JavaValue.(*values.JavaExpression)
				}
				if copied == nil || copied == expr || copied.Op != expr.Op || copied.Values[0] != expr.Values[0] || copied.Values[1] != expr.Values[1] {
					t.Fatal("duplicated step changed value, displacement, or evaluation")
				}
			}
		})
	}
}

func TestComputedLoopStepRejectsEffectsAndIncompleteWordTrees(t *testing.T) {
	for _, variant := range []string{"parameter", "negated parameter", "conditional", "shift", "forwarded update", "cyclic update", "division", "call", "reference", "array", "assignment", "unknown operator", "wrong arity", "cyclic conditional", "cyclic slot", "deep tree", "shared exponential", "wrong result type"} {
		t.Run(variant, func(t *testing.T) {
			integer, boolean := types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaBoolean)
			local := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			parameter.IsParam = true
			flag := values.NewJavaRef(utils.NewRootVariableId(), nil, boolean)
			var amount values.JavaValue = parameter
			want := variant == "parameter" || variant == "negated parameter" || variant == "conditional" || variant == "shift" || variant == "forwarded update"
			switch variant {
			case "negated parameter":
				amount = values.NewUnaryExpression(parameter, values.SUB, integer)
			case "conditional":
				amount = values.NewTernaryExpression(flag, values.NewUnaryExpression(parameter, values.SUB, integer), parameter)
			case "shift":
				amount = values.NewBinaryExpression(parameter, values.NewJavaLiteral(35, integer), values.USHR, integer)
			case "division":
				amount = values.NewBinaryExpression(parameter, local, values.DIV, integer)
			case "call":
				amount = layerTestCall(50)
			case "reference":
				amount = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Integer"))
			case "array":
				amount = &values.JavaArrayMember{}
			case "assignment":
				amount = &values.AssignmentExpression{}
			case "unknown operator":
				amount = values.NewUnaryExpression(parameter, "opaque", integer)
			case "wrong arity":
				amount = values.NewUnaryExpression(parameter, values.MUL, integer)
			case "cyclic conditional":
				cycle := values.NewTernaryExpression(flag, parameter, parameter)
				cycle.TrueValue = cycle
				amount = cycle
			case "cyclic slot":
				cycle := values.NewSlotValue(parameter, integer)
				cycle.ResetValue(cycle)
				amount = cycle
			case "deep tree":
				for i := 0; i < 20; i++ {
					amount = values.NewUnaryExpression(amount, values.SUB, integer)
				}
			case "shared exponential":
				for i := 0; i < 12; i++ {
					amount = values.NewBinaryExpression(amount, amount, values.ADD, integer)
				}
			case "wrong result type":
				amount = flag
			}
			update := values.NewBinaryExpression(local, parameter, values.ADD, integer)
			// Build the host expression before injecting malformed/cyclic
			// operands, so the constructor's type inference is not the test.
			update.Values[1] = amount
			store := statements.NewAssignStatement(local, update, false)
			if variant == "forwarded update" {
				update.Values[0] = values.NewSlotValue(local, integer)
				store.JavaValue = values.NewSlotValue(update, integer)
			}
			if variant == "cyclic update" {
				cycle := values.NewSlotValue(update, integer)
				cycle.ResetValue(cycle)
				store.JavaValue = cycle
			}
			copyStep, ok := switchLoopLocalStep(core.NewNode(store))
			if ok != want {
				t.Fatalf("pure computed step=%v want=%v", ok, want)
			}
			if want {
				copy := copyStep().(*statements.AssignStatement)
				expression := copy.JavaValue.(*values.JavaExpression)
				if copy.LeftValue != local || expression.Values[1] != amount || expression.Values[0] != local {
					t.Fatal("changed the computed displacement or its binding")
				}
			}
		})
	}
}
