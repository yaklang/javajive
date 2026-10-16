package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestIncompleteSourceReceiptDistinguishesLiteralData(t *testing.T) {
	for _, name := range []string{"empty slot", "nil raw value", "nil ternary", "invalid Boolean consumer", "cyclic decision", "literal data"} {
		t.Run(name, func(t *testing.T) {
			state := &class_context.SourceRenderState{}
			ctx := &class_context.ClassContext{SourceRendering: state}
			var text string
			switch name {
			case "empty slot":
				text = (&SlotValue{}).String(ctx)
			case "nil raw value":
				text = javaValueRawString(nil, ctx)
			case "nil ternary":
				text = ternaryRawString(nil, ctx)
			case "invalid Boolean consumer":
				text = NarrowBooleanStackWord(JavaNull).String(ctx)
			case "cyclic decision":
				v := &TernaryExpression{cachedType: types.NewJavaPrimer(types.JavaInteger)}
				v.Condition = v
				v.TrueValue = v
				v.FalseValue = v
				text = v.String(ctx)
			case "literal data":
				text = NewJavaLiteral(EmptySlotValuePlaceholder, types.NewJavaClass("java.lang.String")).String(ctx)
			}
			if state.IncompleteValue != (name != "literal data") {
				t.Fatalf("receipt for %s: %v, %q", name, state.IncompleteValue, text)
			}
			if name != "literal data" && text != EmptySlotValuePlaceholder {
				t.Fatalf("debug text: %q", text)
			}
		})
	}
	if got := (&SlotValue{}).String(nil); got != EmptySlotValuePlaceholder {
		t.Fatal("nil-context debug query")
	}
}
