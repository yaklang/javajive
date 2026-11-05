package core

import (
	"errors"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNumericCopiesRequireEveryOriginalDefinition(t *testing.T) {
	for _, name := range []string{"literal", "copy chain", "increment", "increment loop", "entry int", "entry char", "entry short", "entry byte", "wide parameter layout", "instance parameter", "zero only", "one only", "boolean entry", "reference entry", "long entry", "middle of wide parameter", "receiver", "missing descriptor", "malformed descriptor", "missing entry context", "mixed unknown arm", "empty definitions", "missing original load", "missing decoded load", "missing original store", "missing decoded store", "custom load", "custom store", "wrong source slot", "reference store", "inferred type only", "forged load cell", "duplicate load cell", "load folded to literal", "load folded to producer", "nil operand", "nil literal", "nil slot", "forwarding cycle", "deep forwarding", "boolean literal", "boolean annotation of nonboolean word", "noninteger payload", "malformed load", "malformed store", "malformed increment", "unseeded increment", "unseeded copy cycle", "copy cycle with seed", "bad increment input", "missing graph", "failed graph", "too many original nodes", "too many decoded nodes", "work budget", "allocation budget", "proof budget"} {
		t.Run(name, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			ref.Id.SetName("independent")
			seed, load, target := op(OP_ISTORE_0, 1), op(OP_ILOAD_0, 2), op(OP_ISTORE_2, 3)
			literal := values.NewJavaLiteral(73, integer)
			cell := values.NewSlotValue(ref, integer)
			seed.stackConsumed = []values.JavaValue{literal}
			load.stackProduced = []values.JavaValue{cell}
			target.stackConsumed = []values.JavaValue{cell}
			nodes := []*OpCode{seed, load, target}
			facts := &slotDefinitions{in: map[*OpCode]definitionSet{load: {seed: {}}}}
			graph := &SemanticCFG{Nodes: append([]*OpCode(nil), nodes...), slots: map[int]*slotDefinitions{0: facts}, order: map[*OpCode]int{seed: 0, load: 1, target: 2}}
			d := &Decompiler{semanticCFG: graph, opCodes: nodes, FunctionContext: &class_context.ClassContext{IsStatic: true, CurrentMethodDesc: "(I)V"}}
			producers := map[values.JavaValue]bool{}
			add := func(o *OpCode) {
				d.opCodes = append(d.opCodes, o)
				graph.order[o] = len(graph.Nodes)
				graph.Nodes = append(graph.Nodes, o)
			}
			want := false
			switch name {
			case "literal":
				want = true
			case "copy chain", "copy cycle with seed", "unseeded copy cycle":
				otherLoad, otherStore := op(OP_ILOAD_1, 4), op(OP_ISTORE_1, 5)
				other := values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, integer), integer)
				otherLoad.stackProduced = []values.JavaValue{other}
				seed.stackConsumed = []values.JavaValue{other}
				otherStore.stackConsumed = []values.JavaValue{literal}
				graph.slots[1] = &slotDefinitions{in: map[*OpCode]definitionSet{otherLoad: {otherStore: {}}}}
				add(otherLoad)
				add(otherStore)
				if name != "copy chain" {
					otherStore.stackConsumed = []values.JavaValue{cell}
					if name == "copy cycle with seed" {
						extra := op(OP_ISTORE_0, 6)
						extra.stackConsumed = []values.JavaValue{literal}
						add(extra)
						facts.in[load][extra] = struct{}{}
					}
				}
				want = name != "unseeded copy cycle"
			case "increment", "increment loop", "malformed increment", "unseeded increment", "bad increment input":
				inc := op(OP_IINC, 4)
				inc.Data = []byte{0, 3}
				add(inc)
				facts.in[load] = definitionSet{inc: {}}
				facts.in[inc] = definitionSet{seed: {}}
				seed.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, integer)}
				if name == "increment loop" {
					facts.in[inc][inc] = struct{}{}
				}
				if name == "malformed increment" {
					inc.Data = []byte{0}
				}
				if name == "unseeded increment" {
					facts.in[inc] = definitionSet{inc: {}}
				}
				if name == "bad increment input" {
					facts.in[inc] = definitionSet{nil: {}}
					d.FunctionContext.CurrentMethodDesc = "(Z)V"
				}
				want = name == "increment" || name == "increment loop"
			case "entry int", "entry char", "entry short", "entry byte", "boolean entry", "reference entry", "long entry", "missing descriptor", "malformed descriptor", "missing entry context", "wide parameter layout", "instance parameter", "middle of wide parameter", "receiver":
				facts.in[load] = definitionSet{nil: {}}
				descriptors := map[string]string{"entry int": "(I)V", "entry char": "(C)V", "entry short": "(S)V", "entry byte": "(B)V", "boolean entry": "(Z)V", "reference entry": "(Ljava/lang/Object;)V", "long entry": "(J)V", "missing descriptor": "", "malformed descriptor": "(I)Vjunk", "wide parameter layout": "(JDI)V", "middle of wide parameter": "(J)V"}
				if desc, ok := descriptors[name]; ok {
					d.FunctionContext.CurrentMethodDesc = desc
				}
				if name == "wide parameter layout" {
					load.Instr = op(OP_ILOAD, 2).Instr
					load.Data = []byte{4}
					graph.slots[4] = facts
				}
				if name == "middle of wide parameter" {
					load.Instr = op(OP_ILOAD_1, 2).Instr
					graph.slots[1] = facts
				}
				if name == "instance parameter" || name == "receiver" {
					d.FunctionContext.IsStatic = false
				}
				if name == "instance parameter" {
					load.Instr = op(OP_ILOAD_1, 2).Instr
					graph.slots[1] = facts
				}
				if name == "missing entry context" {
					d.FunctionContext = nil
				}
				want = name == "entry int" || name == "entry char" || name == "entry short" || name == "entry byte" || name == "wide parameter layout" || name == "instance parameter"
			case "zero only":
				literal.Data = 0
			case "one only":
				literal.Data = 1
			case "mixed unknown arm":
				extra := op(OP_ISTORE_0, 4)
				extra.stackConsumed = []values.JavaValue{ref}
				add(extra)
				facts.in[load][extra] = struct{}{}
			case "empty definitions":
				facts.in[load] = definitionSet{}
			case "missing original load":
				graph.Nodes = []*OpCode{seed, target}
			case "missing decoded load":
				d.opCodes = []*OpCode{seed, target}
			case "missing original store":
				graph.Nodes = []*OpCode{load, target}
			case "missing decoded store":
				d.opCodes = []*OpCode{load, target}
			case "custom load":
				load.IsCustom = true
			case "custom store":
				seed.IsCustom = true
			case "wrong source slot":
				seed.Instr = op(OP_ISTORE_1, 1).Instr
			case "reference store":
				seed.Instr = op(OP_ASTORE_0, 1).Instr
			case "inferred type only":
				seed.stackConsumed = []values.JavaValue{ref}
			case "forged load cell":
				target.stackConsumed[0] = values.NewSlotValue(ref, integer)
			case "duplicate load cell":
				other := op(OP_ILOAD_1, 4)
				other.stackProduced = []values.JavaValue{cell}
				add(other)
			case "load folded to literal":
				cell.ResetValue(literal)
				facts.in[load] = definitionSet{}
			case "load folded to producer":
				expression := values.NewBinaryExpression(literal, literal, values.ADD, integer)
				cell.ResetValue(expression)
				producers[expression] = true
				facts.in[load] = definitionSet{}
			case "nil operand":
				target.stackConsumed[0] = nil
			case "nil literal":
				var x *values.JavaLiteral
				seed.stackConsumed[0] = x
			case "nil slot":
				var x *values.SlotValue
				seed.stackConsumed[0] = x
			case "forwarding cycle":
				cyclic := values.NewSlotValue(nil, integer)
				cyclic.ResetValue(cyclic)
				seed.stackConsumed[0] = cyclic
			case "deep forwarding":
				for i := 0; i < 70; i++ {
					seed.stackConsumed[0] = values.NewSlotValue(seed.stackConsumed[0], integer)
				}
			case "boolean literal":
				seed.stackConsumed[0] = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			case "boolean annotation of nonboolean word":
				// The literal domain retains noncanonical JVM int words despite a provisional Z view.
				seed.stackConsumed[0] = values.NewJavaLiteral(73, types.NewJavaPrimer(types.JavaBoolean))
				want = true
			case "noninteger payload":
				seed.stackConsumed[0] = values.NewJavaLiteral(int64(73), integer)
			case "malformed load":
				load.Data = []byte{0}
			case "malformed store":
				seed.Data = []byte{0}
			case "missing graph":
				d.semanticCFG = nil
			case "failed graph":
				graph.Err = errors.New("incomplete")
			case "too many original nodes":
				graph.Nodes = make([]*OpCode, 8193)
			case "too many decoded nodes":
				d.opCodes = make([]*OpCode, 8193)
			case "work budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			proof := d.numericCopyProvenance(producers)
			if name == "proof budget" {
				proof.remaining = 1
			}
			if got := proof.provesStore(target); got != want {
				t.Fatalf("proved=%v want=%v", got, want)
			}
		})
	}
}

func TestNumericCopyJoinPreservesOriginalStoresAndUpdateValue(t *testing.T) {
	for _, name := range []string{"ordinary load", "folded update", "wide locals", "provisional boolean source", "unknown input", "boolean seed"} {
		t.Run(name, func(t *testing.T) {
			integer, boolean := types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaBoolean)
			seed, copied, use := op(OP_ISTORE_2, 10), op(OP_ISTORE_2, 20), op(OP_ILOAD_2, 30)
			original, read, inc := op(OP_ISTORE_0, 1), op(OP_ILOAD_0, 5), op(OP_IINC, 3)
			inc.Data = []byte{0, 6}
			x := values.NewJavaRef(utils.NewRootVariableId(), nil, boolean)
			y := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			source := values.NewJavaRef(utils.NewRootVariableId(), nil, integer)
			zero, word := values.NewJavaLiteral(0, boolean), values.NewJavaLiteral(31, integer)
			original.stackConsumed = []values.JavaValue{word}
			seed.stackConsumed = []values.JavaValue{zero}
			var selected values.JavaValue = values.NewSlotValue(source, integer)
			if name == "folded update" {
				selected = values.NewAssignmentExpression(source, values.NewBinaryExpression(source, values.NewJavaLiteral(6, integer), values.ADD, integer), 3, nil)
			}
			read.stackProduced = []values.JavaValue{selected}
			copied.stackConsumed = []values.JavaValue{values.NewSlotValue(selected, integer)}
			view := values.NewSlotValue(x, boolean)
			use.stackProduced = []values.JavaValue{view}
			nodes := []*OpCode{original, inc, read, seed, copied, use}
			facts := &slotDefinitions{in: map[*OpCode]definitionSet{read: {inc: {}}, inc: {original: {}}}}
			g := &SemanticCFG{Nodes: nodes, slots: map[int]*slotDefinitions{0: facts}, order: map[*OpCode]int{original: 0, inc: 1, read: 2, seed: 3, copied: 4, use: 5}}
			webs := &slotWeb{webOf: map[*OpCode]int{seed: 1, copied: 1, use: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, semanticCFG: g, opCodes: nodes, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{seed: {{x, true}}, copied: {{y, true}}}}
			if name == "wide locals" {
				original.Instr, original.IsWide, original.Data = op(OP_ISTORE, 1).Instr, true, []byte{1, 44}
				read.Instr, read.IsWide, read.Data = op(OP_ILOAD, 5).Instr, true, []byte{1, 44}
				inc.IsWide, inc.Data = true, []byte{1, 44, 0, 6}
				g.slots[300] = facts
			}
			if name == "provisional boolean source" {
				source.ResetVarType(boolean)
			}
			if name == "unknown input" {
				facts.in[inc] = definitionSet{}
			}
			if name == "boolean seed" {
				seed.stackConsumed[0] = values.NewJavaLiteral(false, boolean)
			}
			beforeSeed, beforeCopy := seed.stackConsumed[0], copied.stackConsumed[0]
			d.unifyDescriptorIntJoins()
			got := d.opcodeIdToRef[seed][0][0].(*values.JavaRef)
			want := name != "unknown input" && name != "boolean seed"
			if (got != x) != want {
				t.Fatalf("joined=%v want=%v", got != x, want)
			}
			if want && (d.opcodeIdToRef[copied][0][0] != got || view.GetValue() != got || !isExactPrimer(got.Type(), types.JavaInteger)) {
				t.Fatal("joined declaration and reads disagree")
			}
			if seed.stackConsumed[0] != beforeSeed || copied.stackConsumed[0] != beforeCopy || read.stackProduced[0] != selected || original.stackConsumed[0] != word {
				t.Fatal("changed original definitions, read or folded update")
			}
		})
	}
}
