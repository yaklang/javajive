package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestBooleanStackViewsExposeOnlyTheirTypedOperand(t *testing.T) {
	for _, kind := range []string{"word", "narrow", "forged word", "forged narrow", "missing operand"} {
		t.Run(kind, func(t *testing.T) {
			operand := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			operand.Id.SetName("flag")
			var value JavaValue = booleanStackWord(operand)
			valid := kind == "word" || kind == "narrow"
			switch kind {
			case "narrow":
				operand.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
				value = narrowBooleanLeaf(operand)
			case "forged word", "forged narrow":
				custom := NewCustomValue(func(*class_context.ClassContext) string { return "hidden()" }, operand.Type)
				custom.CapturesKnown = true
				custom.Captures = []JavaValue{operand}
				custom.Flag = "boolean_stack_word"
				if kind == "forged narrow" {
					custom.Flag = "boolean_stack_narrowing"
				}
				value = custom
			case "missing operand":
				value = &booleanStackView{word: true}
			}
			children, known := Children(value)
			if known != valid {
				t.Fatalf("known=%v", known)
			}
			if valid {
				if len(children) != 1 || children[0] != operand {
					t.Fatal("lost operand identity")
				}
				effect, refs := InspectValue(value)
				if effect != 0 || !refs[operand] {
					t.Fatal("lost operand dependency or invented effect")
				}
			}
			if word, known := BooleanStackWordOperand(value); known != (kind == "word") || known && word != operand {
				t.Fatal("numeric domain proof accepted wrong view")
			}
			if kind == "forged word" || kind == "forged narrow" {
				effect, _ := InspectValue(value)
				if effect&EffectOpaque == 0 {
					t.Fatal("closure accepted as sealed arithmetic")
				}
			}
		})
	}
}
func TestBooleanStackViewsPreserveCallEffects(t *testing.T) {
	call := &FunctionCallExpression{FunctionName: "read", Descriptor: "()Z", IsStatic: true, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
	view := booleanStackWord(call)
	effect, _ := InspectValue(view)
	if effect&(EffectCall|EffectReadMemory|EffectWriteMemory|EffectThrow) != (EffectCall|EffectReadMemory|EffectWriteMemory|EffectThrow) || MayFold(view) {
		t.Fatal("operand call effects lost")
	}
}
