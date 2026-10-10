package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeAnonymousInitializerNullArmRequiresExactOriginalProducer(t *testing.T) {
	for _, change := range []string{"original", "unknown PC", "other opcode", "other physical PC", "operand bytes", "missing instruction", "text null", "object literal null", "fake call", "computed null", "nil value", "nil plan"} {
		t.Run(change, func(t *testing.T) {
			op := &core.OpCode{CurrentOffset: 12, Instr: &core.Instruction{OpCode: core.OP_ACONST_NULL}}
			plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{12: op}}
			var value values.JavaValue = values.NewOriginalNullLiteral(12)
			switch change {
			case "unknown PC":
				delete(plan.byPC, 12)
			case "other opcode":
				op.Instr.OpCode = core.OP_ICONST_0
			case "other physical PC":
				op.CurrentOffset = 13
			case "operand bytes":
				op.Data = []byte{0}
			case "missing instruction":
				op.Instr = nil
			case "text null":
				value = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.String"))
			case "object literal null":
				value = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "fake call":
				value = &values.FunctionCallExpression{OriginPC: 12, HasOriginPC: true}
			case "computed null":
				value = &values.JavaExpression{Op: values.EQ, Values: []values.JavaValue{values.JavaNull, values.JavaNull}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
			case "nil value":
				value = nil
			case "nil plan":
				plan = nil
			}
			if known := nativeAnonymousInitializerArmSourceValue(plan, value, 12); known != (change == "original") {
				t.Fatalf("known=%v", known)
			}
		})
	}
}
