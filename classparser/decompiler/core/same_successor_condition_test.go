package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestSameSuccessorConditionRequiresOriginalEdgesAndPrivateEvaluation(t *testing.T) {
	for _, kind := range []string{"field", "local", "call", "two effects", "real branches", "raw different target", "missing branch origin", "missing effect origin", "wrong effect opcode", "shared effect", "same PC clone", "repeated effect", "callback", "ternary", "opaque", "cyclic", "late effect", "synthetic opcode", "inline ref", "shared inline ref", "cyclic inline ref", "typed nil", "lost producer"} {
		t.Run(kind, func(t *testing.T) {
			field := &values.JavaClassMember{OriginPC: 0, HasOriginPC: true, Name: "proof.Guard", Member: "value", Description: "Z", JavaType: types.NewJavaPrimer(types.JavaBoolean)}
			value := values.JavaValue(field)
			pc := 3
			producer := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 0}
			raw := []byte{OP_GETSTATIC, 0, 1, OP_IFEQ, 0, 3, OP_RETURN}
			ops := []*OpCode{producer}
			var originalOperands []values.JavaValue
			if kind == "local" {
				raw[0] = OP_ICONST_1
				producer.Instr.OpCode = OP_ICONST_1
				value = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			}
			if kind == "call" {
				producer.Instr.OpCode = OP_INVOKESTATIC
				raw[0] = OP_INVOKESTATIC
				value = &values.FunctionCallExpression{OriginPC: 0, HasOriginPC: true, IsStatic: true, FunctionName: "evaluate", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
			}
			if kind == "two effects" {
				second := *field
				second.OriginPC = 3
				originalOperands = []values.JavaValue{&second, field}
				value = statements.NewConditionStatement(values.NewJavaCompare(field, &second), EQ).Condition
				pc = 6
				raw = []byte{OP_GETSTATIC, 0, 1, OP_GETSTATIC, 0, 2, OP_IF_ICMPEQ, 0, 3, OP_RETURN}
				ops = append(ops, &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 3})
			}
			statement := &statements.ConditionStatement{Condition: value}
			guard := NewNode(statement)
			guard.Id = 10
			guard.OriginPC = pc
			guard.HasOriginPC = true
			next := NewNode(statements.NewReturnStatement(nil))
			guard.Next = []*Node{next, next}
			next.Source = []*Node{guard, guard}
			opcode := &OpCode{Id: 10, Instr: &Instruction{OpCode: int(raw[pc])}, CurrentOffset: uint16(pc)}
			opcode.stackConsumed = []values.JavaValue{value}
			if originalOperands != nil {
				opcode.stackConsumed = originalOperands
			}
			ops = append(ops, opcode)
			nodes := []*Node{guard, next}
			switch kind {
			case "real branches":
				guard.Next[1] = NewNode(statements.NewReturnStatement(nil))
			case "raw different target":
				raw[pc+2] = 6
			case "missing branch origin":
				guard.HasOriginPC = false
			case "missing effect origin":
				field.HasOriginPC = false
			case "wrong effect opcode":
				producer.Instr.OpCode = OP_GETFIELD
			case "shared effect":
				nodes = append(nodes, NewNode(statements.NewReturnStatement(field)))
			case "same PC clone":
				clone := *field
				nodes = append(nodes, NewNode(statements.NewReturnStatement(&clone)))
			case "repeated effect":
				statement.Condition = statements.NewConditionStatement(values.NewJavaCompare(field, field), EQ).Condition
			case "callback":
				statement.Callback = func(values.JavaValue) {}
			case "ternary":
				statement.TernaryChainArm = true
			case "opaque":
				statement.Condition = &values.CustomValue{}
			case "cyclic":
				expr := &values.JavaExpression{Op: "&&"}
				expr.Values = []values.JavaValue{expr}
				statement.Condition = expr
			case "late effect":
				field.OriginPC = 6
			case "inline ref", "shared inline ref", "cyclic inline ref":
				ref := &values.JavaRef{StackVar: field}
				if kind == "cyclic inline ref" {
					ref.StackVar = ref
				}
				statement.Condition = ref
				value = ref
				opcode.stackConsumed = []values.JavaValue{ref}
				if kind == "shared inline ref" {
					nodes = append(nodes, NewNode(statements.NewReturnStatement(ref)))
				}
			case "typed nil":
				statement.Condition = (*values.JavaExpression)(nil)
			case "lost producer":
				statement.Condition = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			case "synthetic opcode":
				opcode.IsCustom = true
			}
			d := &Decompiler{bytecodes: raw, opCodes: ops}
			d.normalizeSameSuccessorConditions(nodes, map[int]*OpCode{10: opcode})
			positive := kind == "field" || kind == "local" || kind == "call" || kind == "two effects" || kind == "inline ref"
			normalized, ok := guard.Statement.(*statements.IfStatement)
			if ok != positive {
				t.Fatalf("normalized %t want %t", ok, positive)
			}
			if positive {
				if normalized.Condition != value || len(normalized.IfBody) != 0 || len(normalized.ElseBody) != 0 || guard.OriginPC != pc || len(guard.Next) != 1 || guard.Next[0] != next || len(next.Source) != 1 || next.Source[0] != guard {
					t.Fatal("evaluation/PC/continuation ownership changed")
				}
			}
		})
	}
}
