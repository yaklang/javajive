package core

import (
	"errors"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestOriginalEntryLoadRequiresCompleteParameterDefinition(t *testing.T) {
	for _, variant := range []string{"entry", "wide load", "array", "receiver", "original graph", "mixed definitions", "store only", "no definition", "no graph", "failed graph", "not original", "custom load", "malformed load", "primitive load", "primitive parameter", "no witness", "wrong slot", "changed seed", "duplicate witness", "budget"} {
		t.Run(variant, func(t *testing.T) {
			object := types.NewJavaClass("example.Contract")
			seed := values.NewJavaLiteral(nil, object)
			parameter := values.NewJavaRef(utils.NewRootVariableId(), seed, object)
			parameter.IsParam = true
			parameter.MarkOriginalParameter(2)
			load := op(OP_ALOAD_2, 4)
			facts := &slotDefinitions{in: map[*OpCode]definitionSet{load: {nil: {}}}}
			g := &SemanticCFG{Nodes: []*OpCode{load}, order: map[*OpCode]int{load: 0}, slots: map[int]*slotDefinitions{2: facts}}
			// A category-2 predecessor occupies two slots. Witnesses, not
			// parameter slice indices, determine the reference's local slot.
			wide := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaLong))
			d := &Decompiler{semanticCFG: g, Params: []values.JavaValue{wide, parameter}}
			want, slot := false, 2
			switch variant {
			case "entry":
				want = true
			case "wide load":
				load.Instr = op(OP_ALOAD, 4).Instr
				load.IsWide, load.Data = true, []byte{0, 2}
				want = true
			case "array":
				parameter.ResetVarType(types.NewJavaArrayType(object))
				want = true
			case "receiver":
				parameter = values.NewJavaRef(utils.NewRootVariableId(), seed, object)
				parameter.IsParam, parameter.IsThis = true, true
				parameter.MarkOriginalParameter(0)
				parameter.MarkOriginalReceiver()
				d.Params = []values.JavaValue{parameter}
				load.Instr = op(OP_ALOAD_0, 4).Instr
				g.slots, slot = map[int]*slotDefinitions{0: facts}, 0
				want = true
			case "original graph":
				load.Source = []*OpCode{op(OP_ASTORE_2, 50)}
				want = true
			case "mixed definitions":
				facts.in[load][op(OP_ASTORE_2, 1)] = struct{}{}
			case "store only":
				facts.in[load] = definitionSet{op(OP_ASTORE_2, 1): {}}
			case "no definition":
				facts.in[load] = definitionSet{}
			case "no graph":
				d.semanticCFG = nil
			case "failed graph":
				g.Err = errors.New("incomplete")
			case "not original":
				delete(g.order, load)
			case "custom load":
				load.IsCustom = true
			case "malformed load":
				load.Data = []byte{2}
			case "primitive load":
				load.Instr = op(OP_ILOAD_2, 4).Instr
			case "primitive parameter":
				parameter.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "no witness":
				d.Params = []values.JavaValue{values.NewJavaRef(parameter.Id, seed, object)}
			case "wrong slot":
				slot = 1
			case "changed seed":
				parameter.Val = values.NewJavaLiteral(nil, object)
			case "duplicate witness":
				d.Params = append(d.Params, parameter)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			got := d.originalParameterLoadBinding(load, slot)
			if want && got != parameter || !want && got != nil {
				t.Fatalf("binding=%p parameter=%p want=%v", got, parameter, want)
			}
		})
	}
}
