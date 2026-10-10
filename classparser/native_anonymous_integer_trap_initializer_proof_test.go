package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"testing"
)

func TestNativeAnonymousIntegerTrapRequiresExactOriginalEvent(t *testing.T) {
	for _, row := range []struct {
		kind      int
		op, width string
	}{{core.OP_IDIV, values.DIV, types.JavaInteger}, {core.OP_IREM, values.REM, types.JavaInteger}, {core.OP_LDIV, values.DIV, types.JavaLong}, {core.OP_LREM, values.REM, types.JavaLong}} {
		for _, change := range []string{"original", "missing witness", "unknown PC", "other opcode", "other width", "changed operator", "changed operands", "floating escape", "missing type", "missing instruction", "operand bytes", "budget", "cycle"} {
			t.Run(row.width+"/"+row.op+"/"+change, func(t *testing.T) {
				typ := types.NewJavaPrimer(row.width)
				left, right := values.NewJavaLiteral(17, typ), values.NewJavaLiteral(3, typ)
				value := values.NewOriginalIntegerTrapExpression(left, right, row.op, typ, 12)
				op := &core.OpCode{Instr: &core.Instruction{OpCode: row.kind}, CurrentOffset: 12}
				plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{12: op}}
				var work *workbudget.Budget
				switch change {
				case "missing witness":
					value = values.NewBinaryExpression(left, right, row.op, typ)
				case "unknown PC":
					delete(plan.byPC, 12)
				case "other opcode":
					op.Instr.OpCode = core.OP_IADD
				case "other width":
					if row.width == types.JavaInteger {
						op.Instr.OpCode = core.OP_LDIV
					} else {
						op.Instr.OpCode = core.OP_IDIV
					}
				case "changed operator":
					value.Op = values.ADD
				case "changed operands":
					value.Values[0], value.Values[1] = value.Values[1], value.Values[0]
				case "floating escape":
					value.Typ = types.NewJavaPrimer(types.JavaDouble)
				case "missing type":
					value.Typ = nil
				case "missing instruction":
					op.Instr = nil
				case "operand bytes":
					op.Data = []byte{0}
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				case "cycle":
					value.Values[0] = value
				}
				events := []int{}
				known := nativeAnonymousInitializerExpressionEvents(&nativeAnonymousClass{}, plan, value, &events, nil, work)
				if known != (change == "original") {
					t.Fatalf("known=%v events=%v", known, events)
				}
				if known && !reflect.DeepEqual(events, []int{12}) {
					t.Fatalf("events=%v", events)
				}
			})
		}
	}
}

func TestNativeAnonymousIntegerTrapEventsFollowOperandEvaluation(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	literal := func(n int) values.JavaValue { return values.NewJavaLiteral(n, typ) }
	left := values.NewOriginalIntegerTrapExpression(literal(19), literal(3), values.REM, typ, 4)
	right := values.NewOriginalIntegerTrapExpression(literal(13), literal(2), values.DIV, typ, 8)
	root := values.NewOriginalIntegerTrapExpression(left, right, values.DIV, typ, 12)
	plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{}}
	for pc, kind := range map[int]int{4: core.OP_IREM, 8: core.OP_IDIV, 12: core.OP_IDIV} {
		plan.byPC[pc] = &core.OpCode{CurrentOffset: uint16(pc), Instr: &core.Instruction{OpCode: kind}}
	}
	events := []int{}
	if !nativeAnonymousInitializerExpressionEvents(&nativeAnonymousClass{}, plan, root, &events, nil, nil) || !reflect.DeepEqual(events, []int{4, 8, 12}) {
		t.Fatalf("events=%v", events)
	}
}

func TestNativeAnonymousFloatingDivisionDoesNotInventIntegerTrap(t *testing.T) {
	for _, width := range []string{types.JavaFloat, types.JavaDouble} {
		for _, op := range []string{values.DIV, values.REM} {
			typ := types.NewJavaPrimer(width)
			value := values.NewBinaryExpression(values.NewJavaLiteral(1, typ), values.NewJavaLiteral(0, typ), op, typ)
			events := []int{}
			if !nativeAnonymousInitializerExpressionEvents(&nativeAnonymousClass{}, &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{}}, value, &events, nil, nil) || len(events) != 0 {
				t.Fatalf("floating operation introduced integer event: %v", events)
			}
		}
	}
}
