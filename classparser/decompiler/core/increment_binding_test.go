package core

import (
	"errors"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestStandaloneIncrementUsesCompleteOriginalBinding(t *testing.T) {
	for _, variant := range []string{"same category", "reference category", "negative wide", "shared definitions", "increment chain", "original graph only", "distinct definitions", "entry", "unseeded cycle", "empty inputs", "missing ref", "missing decoded seed", "missing original seed", "custom seed", "malformed seed", "wrong slot", "boolean seed", "parameter seed", "post increment", "pre increment", "folded input", "malformed increment", "missing graph", "failed graph", "budget"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			fresh := func(kind types.JavaType) *values.JavaRef {
				return values.NewJavaRef(utils.NewRootVariableId(), nil, kind)
			}
			actual, stale := fresh(integer), fresh(integer)
			actual.Id.SetName("counter")
			stale.Id.SetName("unrelated")
			seed, inc := op(OP_ISTORE_1, 1), op(OP_IINC, 2)
			inc.Data, inc.Ref = []byte{1, 1}, stale
			seed.stackConsumed = []values.JavaValue{values.NewJavaLiteral(17, integer)}
			facts := &slotDefinitions{in: map[*OpCode]definitionSet{inc: {seed: {}, inc: {}}}}
			g := &SemanticCFG{Nodes: []*OpCode{seed, inc}, order: map[*OpCode]int{seed: 0, inc: 1}, slots: map[int]*slotDefinitions{1: facts}}
			d := &Decompiler{semanticCFG: g, opCodes: []*OpCode{seed, inc}, opcodeIdToRef: map[*OpCode][][2]any{seed: {{actual, true}}}}
			add := func(node *OpCode) {
				d.opCodes = append(d.opCodes, node)
				g.order[node] = len(g.Nodes)
				g.Nodes = append(g.Nodes, node)
			}
			want := false
			switch variant {
			case "same category":
				want = true
			case "reference category":
				stale.ResetVarType(types.NewJavaClass("java.lang.Object"))
				want = true
			case "negative wide":
				seed.Instr = op(OP_ISTORE, 1).Instr
				seed.IsWide = true
				seed.Data = []byte{1, 44}
				inc.IsWide = true
				inc.Data = []byte{1, 44, 255, 1}
				g.slots = map[int]*slotDefinitions{300: facts}
				want = true
			case "shared definitions", "distinct definitions":
				other := op(OP_ISTORE_1, 3)
				other.stackConsumed = []values.JavaValue{values.NewJavaLiteral(-9, integer)}
				add(other)
				d.opcodeIdToRef[other] = [][2]any{{actual, false}}
				if variant == "distinct definitions" {
					d.opcodeIdToRef[other][0][0] = fresh(integer)
				} else {
					want = true
				}
				facts.in[inc][other] = struct{}{}
			case "increment chain", "folded input":
				prior := op(OP_IINC, 3)
				prior.Data = []byte{1, 253}
				prior.Ref = actual
				add(prior)
				facts.in[inc] = definitionSet{prior: {}}
				facts.in[prior] = definitionSet{seed: {}, inc: {}}
				prior.IincFoldedToPostOp = variant == "folded input"
				want = variant == "increment chain"
			case "original graph only":
				inc.Source = []*OpCode{op(OP_ASTORE_1, 99)}
				want = true
			case "entry":
				facts.in[inc][nil] = struct{}{}
			case "unseeded cycle":
				facts.in[inc] = definitionSet{inc: {}}
			case "empty inputs":
				facts.in[inc] = definitionSet{}
			case "missing ref":
				delete(d.opcodeIdToRef, seed)
			case "missing decoded seed":
				d.opCodes = []*OpCode{inc}
			case "missing original seed":
				g.Nodes = []*OpCode{inc}
			case "custom seed":
				seed.IsCustom = true
			case "malformed seed":
				seed.Data = []byte{1}
			case "wrong slot":
				seed.Instr = op(OP_ISTORE_2, 1).Instr
			case "boolean seed":
				actual.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "parameter seed":
				actual.IsParam = true
			case "post increment":
				inc.IincFoldedToPostOp = true
			case "pre increment":
				inc.IincFoldedToPreOp = true
			case "malformed increment":
				inc.Data = []byte{1}
			case "missing graph":
				d.semanticCFG = nil
			case "failed graph":
				g.Err = errors.New("incomplete")
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			data := append([]byte(nil), inc.Data...)
			d.restoreStandaloneIncrementBindings()
			if (inc.Ref == actual) != want {
				t.Fatalf("rebound=%v want=%v", inc.Ref == actual, want)
			}
			if !reflect.DeepEqual(data, inc.Data) || d.opcodeIdToRef[seed] != nil && d.opcodeIdToRef[seed][0][0] != actual {
				t.Fatal("rewrote an original definition or signed delta")
			}
			if want {
				protected := false
				for _, ref := range d.disFoldRef {
					protected = protected || ref == actual
				}
				if !protected {
					t.Fatal("read/write binding can be folded into its initializer")
				}
			} else if inc.Ref != stale {
				t.Fatal("incomplete proof changed the binding")
			}
		})
	}
}
