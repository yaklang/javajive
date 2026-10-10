package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdjacentGuardArrayRequiresPrivateOrderedCompleteUse(t *testing.T) {
	for _, kind := range []string{"literal", "primitive", "earlier effect", "same handler", "earlier effect after allocation", "handler change", "alternate opcode entry", "alternate guard entry", "other read", "repeated argument", "effectful element", "local write", "incomplete initializer", "widened view", "callback", "branch definition"} {
		t.Run(kind, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			component := types.JavaType(types.NewJavaClass("java.lang.String"))
			if kind == "primitive" {
				component = integer
			}
			typ := types.NewJavaArrayType(component)
			array := values.NewNewArrayExpression(typ, values.NewJavaLiteral(1, integer))
			array.OriginPC, array.HasOriginPC = 2, true
			array.EvaluationEndPC, array.HasEvaluationEndPC = 3, true
			array.Initializer = []values.JavaValue{values.NewJavaLiteral("label", component)}
			if kind == "primitive" {
				array.Initializer[0] = values.NewJavaLiteral(7, integer)
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), array, typ)
			earlier := &values.FunctionCallExpression{IsStatic: true, FunctionName: "earlier", FuncType: &types.JavaFuncType{ReturnType: integer}}
			argument := values.JavaValue(values.NewJavaLiteral(1, integer))
			if kind == "earlier effect" || kind == "earlier effect after allocation" {
				argument = earlier
			}
			call := &values.FunctionCallExpression{IsStatic: true, FunctionName: "check", OriginPC: 4, HasOriginPC: true, Arguments: []values.JavaValue{argument, ref}, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{integer, typ}, ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
			producer, allocation, fill, invocation := op(OP_INVOKESTATIC, 1), op(OP_ANEWARRAY, 2), op(OP_AASTORE, 3), op(OP_INVOKESTATIC, 4)
			if kind == "primitive" {
				allocation.Instr.OpCode = OP_NEWARRAY
				fill.Instr.OpCode = OP_IASTORE
			}
			producer.stackProduced = []values.JavaValue{earlier}
			all := []*OpCode{producer, allocation, fill, invocation}
			for i := 0; i < len(all)-1; i++ {
				all[i].Target, all[i+1].Source = []*OpCode{all[i+1]}, []*OpCode{all[i]}
			}
			root := NewNode(statements.NewMiddleStatement("start", nil))
			definition := NewNode(statements.NewAssignStatement(ref, array, true))
			condition := &statements.ConditionStatement{Condition: call}
			guard := NewNode(condition)
			root.AddNext(definition)
			definition.AddNext(guard)
			guard.AddNext(NewNode(statements.NewReturnStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)))))
			guard.AddNext(NewNode(statements.NewReturnStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)))))
			guard.TrueNode = func() *Node { return guard.Next[0] }
			guard.FalseNode = func() *Node { return guard.Next[1] }
			d := &Decompiler{RootNode: root, opCodes: all, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}}
			for _, code := range all {
				d.opcodeToSimulateStack[code] = nil
			}
			switch kind {
			case "same handler", "handler change":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 5, HandlerPc: 9}}
				if kind == "handler change" {
					d.ExceptionTable[0].EndPc = 4
				}
			case "earlier effect after allocation":
				producer.CurrentOffset = 5
			case "alternate opcode entry":
				invocation.Source = append(invocation.Source, op(OP_NOP, 10))
			case "alternate guard entry":
				root.AddNext(guard)
			case "other read":
				root.AddNext(NewNode(statements.NewExpressionStatement(ref)))
			case "repeated argument":
				call.Arguments[0] = ref
			case "effectful element":
				array.Initializer[0] = earlier
			case "local write":
				fill.Instr.OpCode = OP_ISTORE
			case "incomplete initializer":
				array.Initializer = nil
			case "widened view":
				ref.WebDeclType = types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
			case "callback":
				condition.Callback = func(values.JavaValue) {}
			case "branch definition":
				definition.AddNext(guard.Next[0])
			}
			want := kind == "literal" || kind == "primitive" || kind == "earlier effect" || kind == "same handler"
			count := d.InlineAdjacentLiteralGuardArrays()
			if (count == 1) != want {
				t.Fatalf("inlined=%d want proof=%t", count, want)
			}
			if want {
				if call.Arguments[1] != array || root.Next[0] != guard || len(definition.Next) != 0 || len(definition.Source) != 0 {
					t.Fatal("proved substitution lost ownership or original edge order")
				}
			} else if call.Arguments[1] != ref || root.Next[0] != definition || definition.Next[0] != guard {
				t.Fatal("rejected substitution changed the call or graph")
			}
		})
	}
}
