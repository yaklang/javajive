package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func primitivePredicateSourceFixture(kind int, op string) (*nativeAnonymousExpressionInitializer, *values.JavaExpression) {
	branch := &core.OpCode{CurrentOffset: 12, Instr: &core.Instruction{OpCode: kind}, Data: []byte{0, 7}}
	call := &values.FunctionCallExpression{OriginPC: 8, HasOriginPC: true, ClassName: "p.Sequence", FunctionName: "size", Descriptor: "()I", FuncType: types.NewJavaFuncType("()I", nil, types.NewJavaPrimer(types.JavaInteger))}
	return &nativeAnonymousExpressionInitializer{branches: map[int]*core.OpCode{8: branch}}, &values.JavaExpression{Op: op, Typ: types.NewJavaPrimer(types.JavaBoolean), Values: []values.JavaValue{call, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}}
}

func TestNativeAnonymousPrimitivePredicateMatchesIndependentSignedModel(t *testing.T) {
	taken := map[int]func(int32) bool{core.OP_IFEQ: func(n int32) bool { return n == 0 }, core.OP_IFNE: func(n int32) bool { return n != 0 }, core.OP_IFLT: func(n int32) bool { return n < 0 }, core.OP_IFGE: func(n int32) bool { return n >= 0 }, core.OP_IFGT: func(n int32) bool { return n > 0 }, core.OP_IFLE: func(n int32) bool { return n <= 0 }}
	source := map[string]func(int32) bool{values.EQ: func(n int32) bool { return n == 0 }, values.NEQ: func(n int32) bool { return n != 0 }, values.LT: func(n int32) bool { return n < 0 }, values.GTE: func(n int32) bool { return n >= 0 }, values.GT: func(n int32) bool { return n > 0 }, values.LTE: func(n int32) bool { return n <= 0 }}
	admitted, worlds := 0, 0
	for kind, vmTest := range taken {
		for op, sourceTest := range source {
			plan, condition := primitivePredicateSourceFixture(kind, op)
			branch, yesFallsThrough := nativeAnonymousInitializerSourceBranch(plan, condition)
			if branch == nil {
				continue
			}
			admitted++
			for _, n := range []int32{-2147483648, -1, 0, 1, 2147483647} {
				got := sourceTest(n)
				if !yesFallsThrough {
					got = !got
				}
				if got == vmTest(n) {
					t.Fatalf("changed selected arm for opcode=%d operator=%s word=%d", kind, op, n)
				}
				worlds++
			}
		}
	}
	if admitted != 12 || worlds != 60 {
		t.Fatalf("predicate census admitted=%d worlds=%d", admitted, worlds)
	}
}

func TestNativeAnonymousPrimitivePredicateRefusesChangedSourceCategory(t *testing.T) {
	for _, variant := range []string{"unknown producer", "missing producer pc", "different producer pc", "nonzero", "long zero", "boxed zero", "boxed operand", "long operand", "boolean operand", "reference branch", "two-word branch", "unrelated operator", "missing operand", "opaque condition"} {
		t.Run(variant, func(t *testing.T) {
			plan, condition := primitivePredicateSourceFixture(core.OP_IFNE, values.EQ)
			call := condition.Values[0].(*values.FunctionCallExpression)
			var value values.JavaValue = condition
			switch variant {
			case "unknown producer":
				delete(plan.branches, 8)
			case "missing producer pc":
				call.HasOriginPC = false
			case "different producer pc":
				call.OriginPC++
			case "nonzero":
				condition.Values[1] = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "long zero":
				condition.Values[1] = values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong))
			case "boxed zero":
				condition.Values[1] = values.NewJavaLiteral(0, types.NewJavaClass("java.lang.Integer"))
			case "boxed operand":
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.Integer")
			case "long operand":
				call.FuncType.ReturnType = types.NewJavaPrimer(types.JavaLong)
			case "boolean operand":
				call.FuncType.ReturnType = types.NewJavaPrimer(types.JavaBoolean)
			case "reference branch":
				plan.branches[8].Instr.OpCode = core.OP_IFNONNULL
			case "two-word branch":
				plan.branches[8].Instr.OpCode = core.OP_IF_ICMPNE
			case "unrelated operator":
				condition.Op = values.ADD
			case "missing operand":
				condition.Values = condition.Values[:1]
			case "opaque condition":
				value = &values.CustomValue{}
			}
			if branch, _ := nativeAnonymousInitializerSourceBranch(plan, value); branch != nil {
				t.Fatal("changed primitive predicate retained certificate")
			}
		})
	}
}

func TestNativeAnonymousCastArmRequiresOriginalCheckedResult(t *testing.T) {
	for _, variant := range []string{"original", "wrong pc", "missing instruction", "wrong opcode", "operand bytes", "binding cast", "unmarked cast", "changed target", "input only"} {
		t.Run(variant, func(t *testing.T) {
			operand := values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))
			cast := values.NewOriginalCheckCast(operand, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaByte)), 17)
			plan := &nativeAnonymousExpressionInitializer{byPC: map[int]*core.OpCode{17: {CurrentOffset: 17, Instr: &core.Instruction{OpCode: core.OP_CHECKCAST}, Data: []byte{0, 1}}}}
			var value values.JavaValue = cast
			switch variant {
			case "wrong pc":
				cast.OriginPC++
			case "missing instruction":
				plan.byPC[17].Instr = nil
			case "wrong opcode":
				plan.byPC[17].Instr.OpCode = core.OP_ACONST_NULL
			case "operand bytes":
				plan.byPC[17].Data = []byte{0}
			case "binding cast":
				cast.Binding = true
			case "unmarked cast":
				cast.OriginalCheckCast = false
			case "changed target":
				cast.TargetType = types.NewJavaClass("java.lang.String")
			case "input only":
				value = operand
			}
			if got := nativeAnonymousInitializerArmSourceValue(plan, value, 17); got != (variant == "original") {
				t.Fatalf("cast arm=%v", got)
			}
		})
	}
}
