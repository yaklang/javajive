package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestMixedBooleanBitwiseKeepsIntegerParameterABI(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, op := range []string{AND, OR, XOR} {
		for _, reverse := range []bool{false, true} {
			word := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			word.IsParam = true
			word.Id.SetName("word")
			flag := NewRefMember(NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner")), "flag", types.NewJavaPrimer(types.JavaBoolean))
			left, right := JavaValue(flag), JavaValue(word)
			if reverse {
				left, right = right, left
			}
			expr := NewBinaryExpression(left, right, op, left.Type())
			if expr.Type().String(ctx) != "int" || word.Type().String(ctx) != "int" || flag.Type().String(ctx) != "boolean" {
				t.Fatal("mixed bitwise operation changed the parameter/field ABI")
			}
			text := expr.String(ctx)
			if strings.Count(text, "? 1 : 0") != 1 || strings.Count(text, word.String(ctx)) != 1 {
				t.Fatal(text)
			}
			if !IsBooleanStackNarrowing(types.NewJavaPrimer(types.JavaBoolean), expr) || strings.Count(NarrowBooleanStackWord(expr).String(ctx), "& 1") != 1 {
				t.Fatal("missing bit-zero sink conversion")
			}
		}
	}
}

func TestLateMixedBooleanBitwiseKeepsNumericWord(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, op := range []string{AND, OR, XOR} {
		for _, reverse := range []bool{false, true} {
			word := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			word.Id.SetName("word")
			flag := NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			operands := []JavaValue{word, flag}
			if reverse {
				operands[0], operands[1] = operands[1], operands[0]
			}
			expr := NewBinaryExpression(operands[0], operands[1], op, word.Type())
			flag.Type().ResetType(types.NewJavaPrimer(types.JavaBoolean))
			text := expr.String(ctx)
			if strings.Count(text, "? 1 : 0") != 1 || strings.Count(text, "word") != 1 || expr.Type().String(ctx) != "int" || word.Type().String(ctx) != "int" || flag.Type().String(ctx) != "boolean" {
				t.Fatal(text)
			}
		}
	}
}

func TestBooleanStackNarrowingRequiresComputationalInt(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, name := range []string{types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaBoolean, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		value := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(name))
		want := name == types.JavaByte || name == types.JavaShort || name == types.JavaChar || name == types.JavaInteger
		if IsBooleanStackNarrowing(types.NewJavaPrimer(types.JavaBoolean), value) != want {
			t.Fatal(name)
		}
		if value.Type().String(ctx) != name {
			t.Fatal("mutated producer")
		}
		if IsBooleanStackNarrowing(types.NewJavaPrimer(types.JavaInteger), value) {
			t.Fatal("numeric sink narrowed")
		}
	}
	if IsBooleanStackNarrowing(nil, nil) {
		t.Fatal("missing evidence accepted")
	}
}

func TestBooleanConsumerViewFollowsProvedLateProducerDomain(t *testing.T) {
	ctx := &class_context.ClassContext{}
	word := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	word.Id.SetName("word")
	view, ok := BooleanStackConsumerView(word)
	if !ok || !strings.Contains(view.String(ctx), "& 1") {
		t.Fatal("unproved numeric word lost low-bit narrowing")
	}
	word.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
	if text := view.String(ctx); text != "word" {
		t.Fatal("late proved boolean source received integer arithmetic: " + text)
	}
	word.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
	if text := view.String(ctx); !strings.Contains(text, "& 1") || strings.Count(text, "word") != 1 {
		t.Fatal("view cached a producer domain or duplicated an evaluation: " + text)
	}
}

func TestPrimitiveABIAndNumericProducerDoNotAdoptConsumerType(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, name := range []string{types.JavaBoolean, types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		param := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(name))
		param.IsParam = true
		param.Type().ResetType(types.NewJavaPrimer(types.JavaBoolean))
		if param.Type().String(ctx) != name {
			t.Fatalf("parameter descriptor mutated: %s", name)
		}
	}
	for _, op := range []string{ADD, SUB, MUL, DIV, REM, AND, OR, XOR, SHL, SHR, USHR} {
		word := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
		word.IsParam = true
		expr := NewBinaryExpression(word, NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger)), op, word.Type())
		expr.Type().ResetType(types.NewJavaPrimer(types.JavaBoolean))
		if expr.Type().String(ctx) != "int" {
			t.Fatal("numeric producer mutated by boolean consumer: " + op)
		}
	}
}

func TestLateBooleanWebKeepsConstantViewAndNumericInvocationABI(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, op := range []string{AND, OR, XOR} {
		for _, bit := range []int{0, 1, 2} {
			local := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			local.Id.SetName("flag")
			literal := NewJavaLiteral(bit, types.NewJavaPrimer(types.JavaInteger))
			expression := NewBinaryExpression(local, literal, op, local.Type())
			local.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			_, _, normalized := expression.boolConnectiveConds()
			if normalized != (bit == 0 || bit == 1) {
				t.Fatalf("op%s bit%d normalized%v", op, bit, normalized)
			}
			if literal.Type().String(ctx) != "int" {
				t.Fatal("late rendering mutated shared constant")
			}
			if normalized && expression.Type().String(ctx) != "boolean" {
				t.Fatal("proved normalized connective lost domain")
			}
		}
	}
	flag := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
	flag.IsParam = true
	flag.Id.SetName("flag")
	call := &FunctionCallExpression{Descriptor: "(Z)Z", Arguments: []JavaValue{flag}, IsStatic: true, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{flag.Type()}, ReturnType: flag.Type()}}
	if call.ArgumentStrings(ctx)[0] != "flag" {
		t.Fatal("boolean invocation converted")
	}
	call.Descriptor = "(I)Z"
	if text := call.ArgumentStrings(ctx)[0]; strings.Count(text, "? 1 : 0") != 1 || strings.Count(text, "flag") != 1 {
		t.Fatal(text)
	}
}

func TestStackLifetimeConsumerViewsFollowOriginalLoadTypeAndBinding(t *testing.T) {
	ctx := &class_context.ClassContext{}
	integer := types.NewJavaPrimer(types.JavaInteger)
	first := NewJavaRef(utils.NewRootVariableId(), nil, integer)
	first.Id.SetName("word")
	original := NewSlotValue(first, integer)
	view := NewStackLifetimeUseView(original)
	if view.TmpType != nil || OriginalStackLifetimeUse(view) != original {
		t.Fatal("consumer invented a type or lost the original load")
	}
	solved := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
	solved.Id.SetName("flag")
	solved.WebDeclType = solved.Type().Copy()
	original.ResetValue(solved)
	consumer, ok := BooleanStackConsumerView(view)
	if !ok || consumer.String(ctx) != "flag" || first.Type().String(ctx) != "int" {
		t.Fatal("late binding changed original alias or inserted numeric conversion")
	}
	saved := NewJavaRef(utils.NewRootVariableId(), nil, integer.Copy())
	view.ResetValue(saved)
	if OriginalStackLifetimeUse(view) != saved || saved.Type().String(ctx) != "int" {
		t.Fatal("immutable copy adopted a consumer type")
	}
	consumer, ok = BooleanStackConsumerView(view)
	if !ok || !strings.Contains(consumer.String(ctx), "& 1") {
		t.Fatal("noncanonical numeric word lost low-bit narrowing")
	}
	var deep JavaValue = original
	for i := 0; i < 31; i++ {
		deep = NewStackLifetimeUseView(deep)
	}
	if OriginalStackLifetimeUse(deep) != original {
		t.Fatal("bounded forwarding did not retain original slot")
	}
	deep = NewStackLifetimeUseView(deep)
	if OriginalStackLifetimeUse(deep) != nil {
		t.Fatal("forwarding bound exceeded")
	}
	ordinary := NewSlotValue(original, integer)
	if OriginalStackLifetimeUse(ordinary) != ordinary {
		t.Fatal("ordinary alias acquired forwarding proof")
	}
}
