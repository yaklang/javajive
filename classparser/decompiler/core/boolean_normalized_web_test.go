package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNormalizedBooleanWebRequiresClosedDefinitionsAndBooleanUses(t *testing.T) {
	for _, kind := range []string{"proved", "nonboolean seed", "numeric consumer", "increment", "wide increment", "parameter", "entry", "foreign owner", "missing definition", "nonboolean operand", "diagnostic off"} {
		t.Run(kind, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			zero := values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			predicate := &values.FunctionCallExpression{ClassName: "Probe", FunctionName: "flag", Descriptor: "()Z", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
			expr := values.NewBinaryExpression(ref, predicate, values.OR, ref.Type())
			seed, store, load, bit, ret := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 5), op(OP_ILOAD_1, 2), op(OP_IOR, 4), op(OP_IRETURN, 7)
			seed.stackConsumed, store.stackConsumed = []values.JavaValue{zero}, []values.JavaValue{expr}
			bit.stackConsumed, ret.stackConsumed = []values.JavaValue{predicate, ref}, []values.JavaValue{ref}
			webs := &slotWeb{webOf: map[*OpCode]int{seed: 1, store: 1, load: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{cachedSlotWebs: webs, opCodes: []*OpCode{seed, load, bit, store, ret}, opcodeIdToRef: map[*OpCode][][2]any{seed: {{ref, true}}, store: {{ref, false}}}, FunctionType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
			switch kind {
			case "nonboolean seed":
				zero.Data = 2
			case "numeric consumer":
				ret.Instr.OpCode = OP_IADD
			case "increment", "wide increment":
				inc := op(OP_IINC, 8)
				inc.Data = []byte{1, 1}
				inc.Source = []*OpCode{store}
				if kind == "wide increment" {
					inc.IsWide, inc.Data = true, []byte{0, 1, 0, 1}
				}
				d.opCodes = append(d.opCodes, inc)
			case "parameter":
				ref.IsParam = true
			case "entry":
				webs.entryWeb[0] = 1
			case "foreign owner":
				other := op(OP_ISTORE_1, 8)
				webs.webOf[other] = 2
				d.opcodeIdToRef[other] = [][2]any{{ref, true}}
			case "missing definition":
				store.stackConsumed = nil
			case "nonboolean operand":
				expr.Values[1] = values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
			case "diagnostic off":
				d.Env = func(string) string { return "1" }
			}
			d.FunctionContext = &class_context.ClassContext{FunctionType: d.FunctionType}
			d.restoreNormalizedBooleanWebs()
			if got := isExactPrimer(ref.Type(), types.JavaBoolean); got != (kind == "proved") {
				t.Fatalf("recovered=%v", got)
			}
			if kind == "proved" && !isExactPrimer(expr.Type(), types.JavaBoolean) {
				t.Fatal("lost boolean recurrence")
			}
		})
	}
}

func TestNormalizedBooleanWebJoinsSplitSourceNamesWithoutCrossWebMutation(t *testing.T) {
	for _, kind := range []string{"split", "already boolean seed", "noncanonical", "numeric use", "foreign store", "foreign load", "missing load", "unreplaceable load", "missing definition"} {
		t.Run(kind, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			first := values.NewJavaRef(utils.NewRootVariableId(), nil, integer.Copy())
			second := values.NewJavaRef(utils.NewRootVariableId(), nil, integer.Copy())
			first.Id.SetName("flag")
			second.Id.SetName("other")
			zero, one := values.NewJavaLiteral(0, integer.Copy()), values.NewJavaLiteral(1, integer.Copy())
			seed, store, load, branch := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 5), op(OP_ILOAD_1, 7), op(OP_IFNE, 8)
			seed.stackConsumed, store.stackConsumed = []values.JavaValue{zero}, []values.JavaValue{one}
			view := values.NewSlotValue(first, integer.Copy())
			load.stackProduced, branch.stackConsumed = []values.JavaValue{view}, []values.JavaValue{view}
			webs := &slotWeb{webOf: map[*OpCode]int{seed: 1, store: 1, load: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{cachedSlotWebs: webs, opCodes: []*OpCode{seed, store, load, branch}, opcodeIdToRef: map[*OpCode][][2]any{seed: {{first, true}}, store: {{second, true}}}}
			switch kind {
			case "already boolean seed":
				zero.JavaType = types.NewJavaPrimer(types.JavaBoolean)
			case "noncanonical":
				one.Data = 2
			case "numeric use":
				branch.Instr.OpCode = OP_IADD
			case "foreign store":
				foreign := op(OP_ISTORE_1, 10)
				webs.webOf[foreign] = 2
				d.opcodeIdToRef[foreign] = [][2]any{{second, true}}
			case "foreign load":
				webs.webOf[load] = 2
			case "missing load":
				load.stackProduced = nil
			case "unreplaceable load":
				load.stackProduced[0] = first
			case "missing definition":
				webs.webOf[op(OP_ISTORE_1, 10)] = 1
			}
			d.restoreNormalizedBooleanWebs()
			got := d.opcodeIdToRef[seed][0][0].(*values.JavaRef)
			want := kind == "split" || kind == "already boolean seed"
			if (got != first) != want {
				t.Fatalf("joined=%v want=%v", got != first, want)
			}
			if !isExactPrimer(first.Type(), types.JavaInteger) || !isExactPrimer(second.Type(), types.JavaInteger) {
				t.Fatal("mutated original simulator aliases globally")
			}
			if want && (!isExactPrimer(got.Type(), types.JavaBoolean) || d.opcodeIdToRef[store][0][0] != got || d.opcodeIdToRef[store][0][1] != false || view.GetValue() != got || got.SolvedWebIdentity != got.Id || webs.webOf[seed] != 1 || webs.webOf[load] != 1) {
				t.Fatal("lost source declaration, replaceable load or immutable web identity")
			}
		})
	}
}
