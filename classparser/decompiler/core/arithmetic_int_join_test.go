package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNativeArithmeticJoinRequiresOriginalProducerAndClosedWeb(t *testing.T) {
	for _, instruction := range []struct {
		code     int
		operator string
	}{
		{OP_IADD, values.ADD}, {OP_ISUB, values.SUB}, {OP_IMUL, values.MUL},
		{OP_IDIV, values.DIV}, {OP_IREM, values.REM},
		{OP_ISHL, values.SHL}, {OP_ISHR, values.SHR}, {OP_IUSHR, values.USHR},
	} {
		for _, change := range []string{"proved", "consumer view", "wrong opcode", "wrong operator", "different operand", "missing operand", "extra operand", "missing result", "copied result", "wrong type", "missing trap origin", "drifted trap origin", "unknown definition", "missing load", "entry"} {
			t.Run(instruction.operator+"/"+change, func(t *testing.T) {
				integer := types.NewJavaPrimer(types.JavaInteger)
				seed, producer, store, load := op(OP_ISTORE_1, 1), op(instruction.code, 4), op(OP_ISTORE_1, 5), op(OP_ILOAD_1, 6)
				first := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
				first.Id.SetName("seed")
				second := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
				second.Id.SetName("computed")
				left, right := values.NewJavaLiteral(79, integer), values.NewJavaLiteral(3, integer)
				expression := values.NewBinaryExpression(left, right, instruction.operator, integer)
				trap := instruction.code == OP_IDIV || instruction.code == OP_IREM
				if trap {
					expression = values.NewOriginalIntegerTrapExpression(left, right, instruction.operator, integer, 4)
				}
				seed.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaBoolean))}
				producer.stackConsumed, producer.stackProduced = []values.JavaValue{right, left}, []values.JavaValue{expression}
				store.stackConsumed = []values.JavaValue{expression}
				view := values.NewSlotValue(first, first.Type())
				load.stackProduced = []values.JavaValue{view}
				webs := &slotWeb{webOf: map[*OpCode]int{seed: 1, store: 1, load: 1}, entryWeb: map[int]int{}}
				d := &Decompiler{opCodes: []*OpCode{seed, producer, store, load}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{seed: {{first, true}}, store: {{second, true}}}}
				switch change {
				case "consumer view":
					store.stackConsumed[0] = values.NewStackLifetimeUseView(expression)
				case "wrong opcode":
					producer.Instr = op(OP_LADD, 4).Instr
				case "wrong operator":
					expression.Op = values.XOR
				case "different operand":
					producer.stackConsumed[0] = values.NewJavaLiteral(3, integer)
				case "missing operand":
					producer.stackConsumed = nil
				case "extra operand":
					producer.stackConsumed = append(producer.stackConsumed, left)
				case "missing result":
					producer.stackProduced = nil
				case "copied result":
					producer.stackProduced[0] = values.NewBinaryExpression(left, right, instruction.operator, integer)
				case "wrong type":
					expression.Typ = types.NewJavaPrimer(types.JavaLong)
				case "missing trap origin":
					if trap {
						expression = values.NewBinaryExpression(left, right, instruction.operator, integer)
						producer.stackProduced[0], store.stackConsumed[0] = expression, expression
					}
				case "drifted trap origin":
					producer.CurrentOffset = 10
				case "unknown definition":
					webs.webOf[op(OP_ISTORE_1, 9)] = 1
				case "missing load":
					load.stackProduced = nil
				case "entry":
					webs.entryWeb[1] = 1
				}
				d.unifyDescriptorIntJoins()
				got := d.opcodeIdToRef[seed][0][0].(*values.JavaRef)
				want := change == "proved" || change == "consumer view" || (!trap && (change == "missing trap origin" || change == "drifted trap origin"))
				if (got != first) != want {
					t.Fatalf("rebound=%v want=%v", got != first, want)
				}
				if want && (view.GetValue() != got || d.opcodeIdToRef[store][0][0] != got || got.SolvedWebIdentity != got.Id || !isExactPrimer(got.Type(), types.JavaInteger)) {
					t.Fatal("definition/load identity lost")
				}
				if !isExactPrimer(first.Type(), types.JavaBoolean) {
					t.Fatal("mutated unrelated simulator alias")
				}
			})
		}
	}
}
