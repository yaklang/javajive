package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strconv"
	"testing"
)

func TestNumericBitwiseRequiresBothOriginalOperandDomains(t *testing.T) {
	for _, code := range []int{OP_IAND, OP_IOR, OP_IXOR} {
		for _, name := range []string{"integer", "byte", "short", "char", "boolean", "reference", "nested", "constant seed", "boolean literal", "unknown operand", "foreign operand", "swapped operands", "wrong expression", "partial expression", "nil expression", "nil operand", "typed nil operand", "custom", "wide", "payload", "partial inputs", "partial outputs", "missing original", "duplicate result", "mutable type only", "forward limit", "proof cap"} {
			t.Run(strconv.Itoa(code)+"/"+name, func(t *testing.T) {
				integer := types.NewJavaPrimer(types.JavaInteger)
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
				cell := values.NewSlotValue(ref, integer)
				constant := values.NewJavaLiteral(255, integer)
				load, bits, store := op(OP_ILOAD_0, 0), op(code, 1), op(OP_ISTORE_1, 2)
				operator := values.AND
				if code == OP_IOR {
					operator = values.OR
				}
				if code == OP_IXOR {
					operator = values.XOR
				}
				x := values.NewBinaryExpression(cell, constant, operator, integer)
				load.stackProduced = []values.JavaValue{cell}
				bits.stackConsumed = []values.JavaValue{constant, cell}
				bits.stackProduced = []values.JavaValue{x}
				store.stackConsumed = []values.JavaValue{x}
				g := &SemanticCFG{Nodes: []*OpCode{load, bits, store}, order: map[*OpCode]int{load: 0, bits: 1, store: 2}, slots: map[int]*slotDefinitions{0: {in: map[*OpCode]definitionSet{load: {nil: {}}}}}}
				d := &Decompiler{opCodes: append([]*OpCode(nil), g.Nodes...), semanticCFG: g, FunctionContext: &class_context.ClassContext{IsStatic: true, CurrentMethodDesc: "(I)V"}}
				want := name == "integer" || name == "byte" || name == "short" || name == "char" || name == "nested" || name == "constant seed"
				switch name {
				case "byte":
					d.FunctionContext.CurrentMethodDesc = "(B)V"
				case "short":
					d.FunctionContext.CurrentMethodDesc = "(S)V"
				case "char":
					d.FunctionContext.CurrentMethodDesc = "(C)V"
				case "boolean":
					d.FunctionContext.CurrentMethodDesc = "(Z)V"
				case "reference":
					d.FunctionContext.CurrentMethodDesc = "(Ljava/lang/Object;)V"
				case "nested":
					outer := op(OP_IXOR, 3)
					zero := values.NewJavaLiteral(0, integer)
					y := values.NewBinaryExpression(x, zero, values.XOR, integer)
					outer.stackConsumed = []values.JavaValue{zero, x}
					outer.stackProduced = []values.JavaValue{y}
					store.stackConsumed[0] = y
					g.order[outer] = 3
					g.Nodes = append(g.Nodes, outer)
					d.opCodes = append(d.opCodes, outer)
				case "constant seed":
					v := values.NewJavaLiteral(7, integer)
					bits.stackConsumed[1] = v
					x.Values[0] = v
				case "boolean literal":
					constant.Data = true
				case "unknown operand", "mutable type only":
					bits.stackConsumed[1] = ref
					x.Values[0] = ref
				case "foreign operand":
					x.Values[0] = ref
				case "swapped operands":
					x.Values[0], x.Values[1] = x.Values[1], x.Values[0]
				case "wrong expression":
					x.Op = values.ADD
				case "partial expression":
					x.Values = x.Values[:1]
				case "nil expression":
					bits.stackProduced[0] = (*values.JavaExpression)(nil)
				case "nil operand":
					bits.stackConsumed[1] = nil
					x.Values[0] = nil
				case "typed nil operand":
					var nilValue *values.JavaLiteral
					bits.stackConsumed[1] = nilValue
					x.Values[0] = nilValue
				case "custom":
					bits.IsCustom = true
				case "wide":
					bits.IsWide = true
				case "payload":
					bits.Data = []byte{0}
				case "partial inputs":
					bits.stackConsumed = bits.stackConsumed[:1]
				case "partial outputs":
					bits.stackProduced = append(bits.stackProduced, constant)
				case "missing original":
					g.Nodes = []*OpCode{load, store}
					delete(g.order, bits)
				case "duplicate result":
					duplicate := *bits
					d.opCodes = append(d.opCodes, &duplicate)
					g.Nodes = append(g.Nodes, &duplicate)
					g.order[&duplicate] = 3
				case "forward limit":
					for i := 0; i < 33; i++ {
						x.Values[0] = values.NewStackLifetimeUseView(x.Values[0])
					}
					bits.stackConsumed[1] = x.Values[0]
				}
				p := d.numericCopyProvenance(map[values.JavaValue]bool{})
				if name == "proof cap" {
					p.remaining = 0
				}
				got := p.provesStore(store)
				if got != want {
					t.Fatalf("numeric=%v want=%v", got, want)
				}
			})
		}
	}
}
