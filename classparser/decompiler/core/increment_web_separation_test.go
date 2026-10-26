package core

import (
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNumericWebIgnoresOnlyProvedDisjointIncrementLifetimes(t *testing.T) {
	for _, variant := range []string{"separate", "transitive", "same web", "entry", "unknown definition", "empty inputs", "missing graph", "failed graph", "missing original increment", "missing decoded increment", "missing original definition", "missing original target", "custom increment", "custom definition", "wrong slot", "malformed increment", "budget"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			boolean := types.NewJavaPrimer(types.JavaBoolean)
			seed, producer, store, load, later, inc := op(OP_ISTORE_1, 1), op(OP_IADD, 2), op(OP_ISTORE_1, 3), op(OP_ILOAD_1, 4), op(OP_ISTORE_1, 5), op(OP_IINC, 6)
			inc.Data = []byte{1, 1}
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, boolean)
			old.Id.SetName("carry")
			update := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			lhs, rhs := values.NewJavaLiteral(71, integer), values.NewJavaLiteral(3, integer)
			expression := values.NewBinaryExpression(lhs, rhs, values.ADD, integer)
			producer.stackConsumed = []values.JavaValue{rhs, lhs}
			producer.stackProduced = []values.JavaValue{expression}
			seed.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, boolean)}
			store.stackConsumed = []values.JavaValue{expression}
			later.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, integer)}
			current := values.NewSlotValue(old, boolean)
			load.stackProduced = []values.JavaValue{current}
			nodes := []*OpCode{seed, producer, store, load, later, inc}
			facts := &slotDefinitions{in: map[*OpCode]definitionSet{inc: {later: {}}}}
			graph := &SemanticCFG{Nodes: append([]*OpCode(nil), nodes...), slots: map[int]*slotDefinitions{1: facts}, order: map[*OpCode]int{seed: 0, producer: 1, store: 2, load: 3, later: 4, inc: 5}}
			webs := &slotWeb{webOf: map[*OpCode]int{seed: 1, store: 1, load: 1, later: 2}, entryWeb: map[int]int{}}
			d := &Decompiler{semanticCFG: graph, cachedSlotWebs: webs, opCodes: nodes, opcodeIdToRef: map[*OpCode][][2]any{seed: {{old, true}}, store: {{update, true}}, later: {{old, false}}}}
			switch variant {
			case "transitive":
				second := op(OP_IINC, 7)
				second.Data = []byte{1, 2}
				d.opCodes = append(d.opCodes, second)
				graph.Nodes = append(graph.Nodes, second)
				graph.order[second] = 6
				facts.in[second] = definitionSet{inc: {}}
			case "same web":
				facts.in[inc] = definitionSet{store: {}}
			case "entry":
				facts.in[inc] = definitionSet{nil: {}}
			case "unknown definition":
				delete(webs.webOf, later)
			case "empty inputs":
				facts.in[inc] = definitionSet{}
			case "missing graph":
				d.semanticCFG = nil
			case "failed graph":
				graph.Err = errors.New("incomplete")
			case "missing original increment":
				graph.Nodes = graph.Nodes[:5]
			case "missing decoded increment":
				d.opCodes = d.opCodes[:5]
			case "missing original definition":
				graph.Nodes = []*OpCode{seed, producer, store, load, inc}
			case "missing original target":
				graph.Nodes = graph.Nodes[1:]
			case "custom increment":
				inc.IsCustom = true
			case "custom definition":
				later.IsCustom = true
			case "wrong slot":
				later.Instr = op(OP_ISTORE_2, 5).Instr
			case "malformed increment":
				inc.Data = []byte{1}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			d.unifyDescriptorIntJoins()
			got := d.opcodeIdToRef[seed][0][0].(*values.JavaRef)
			want := variant == "separate" || variant == "transitive"
			if (got != old) != want {
				t.Fatalf("joined=%v want=%v", got != old, want)
			}
			if want && (d.opcodeIdToRef[store][0][0] != got || current.GetValue() != got || d.opcodeIdToRef[later][0][0] != old || !isExactPrimer(got.Type(), types.JavaInteger) || !isExactPrimer(old.Type(), types.JavaBoolean) || store.stackConsumed[0] != expression) {
				t.Fatal("changed another lifetime or lost original numeric producer")
			}
		})
	}
}
