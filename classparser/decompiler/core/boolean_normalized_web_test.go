package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNormalizedBooleanWebRequiresClosedDefinitionsAndBooleanUses(t *testing.T) {
	for _, kind := range []string{"proved", "nonboolean seed", "numeric consumer", "no boolean consumer", "increment", "wide increment", "parameter", "entry", "foreign owner", "missing definition", "nonboolean operand", "diagnostic off"} {
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
			case "no boolean consumer":
				d.opCodes = []*OpCode{seed, store}
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

func TestBooleanCopyComponentsKeepDistinctLocalsAndRejectNumericEscapes(t *testing.T) {
	for _, consumerViews := range []bool{false, true} {
		for _, scenario := range []string{"closed", "noncanonical root", "numeric sink", "entry member", "parameter member", "foreign load", "missing snapshot", "cycle without root"} {
			t.Run(scenario, func(t *testing.T) {
				const length = 32
				refs := make([]*values.JavaRef, length)
				loads := make([]*OpCode, length)
				stores := make([]*OpCode, length)
				views := make([]*values.SlotValue, length)
				webs := &slotWeb{webOf: map[*OpCode]int{}, entryWeb: map[int]int{}}
				d := &Decompiler{cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{}, FunctionType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
				d.FunctionContext = &class_context.ClassContext{FunctionType: d.FunctionType}
				root := values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
				for i := 0; i < length; i++ {
					refs[i] = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
					stores[i], loads[i] = op(OP_ISTORE, uint16(i*4+1)), op(OP_ILOAD, uint16(i*4+3))
					stores[i].Data, loads[i].Data = []byte{byte(i + 1)}, []byte{byte(i + 1)}
					views[i] = values.NewSlotValue(refs[i], types.NewJavaPrimer(types.JavaInteger))
					loads[i].stackProduced = []values.JavaValue{views[i]}
					var rhs values.JavaValue = root
					if i > 0 {
						rhs = views[i-1]
					}
					if consumerViews && i > 0 {
						rhs = values.NewStackLifetimeUseView(rhs)
					}
					stores[i].stackConsumed = []values.JavaValue{rhs}
					// Reverse IDs force type dependencies opposite to processing order.
					webs.webOf[stores[i]], webs.webOf[loads[i]] = length-i, length-i
					d.opcodeIdToRef[stores[i]] = [][2]any{{refs[i], true}}
					d.opCodes = append(d.opCodes, stores[i], loads[i])
				}
				ret := op(OP_IRETURN, length*4+1)
				var returnValue values.JavaValue = views[length-1]
				if consumerViews {
					returnValue = values.NewStackLifetimeUseView(returnValue)
				}
				ret.stackConsumed = []values.JavaValue{returnValue}
				d.opCodes = append(d.opCodes, ret)
				switch scenario {
				case "noncanonical root":
					root.Data = 2
				case "numeric sink":
					ret.Instr.OpCode = OP_IADD
				case "entry member":
					webs.entryWeb[1] = webs.webOf[stores[7]]
				case "parameter member":
					refs[7].IsParam = true
				case "foreign load":
					webs.webOf[loads[7]] = length + 1
				case "missing snapshot":
					loads[7].stackProduced = nil
				case "cycle without root":
					stores[0].stackConsumed = []values.JavaValue{views[length-1]}
				}
				d.restoreNormalizedBooleanWebs()
				for i, ref := range refs {
					if isExactPrimer(ref.Type(), types.JavaBoolean) != (scenario == "closed") {
						t.Fatalf("member %d domain=%s", i, ref.Type().String(&class_context.ClassContext{}))
					}
					if views[i].GetValue() != ref || d.opcodeIdToRef[stores[i]][0][0] != ref {
						t.Fatal("type closure merged the identities of distinct copy locals")
					}
				}
			})
		}
	}
}
