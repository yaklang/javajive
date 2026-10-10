package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func phiTypeHierarchy(name string) ([]string, bool) {
	edges := map[string][]string{"phi/Left": {"phi/Base"}, "phi/Right": {"phi/Base"}, "phi/Base": {"java/lang/Object"}, "phi/Other": {"java/lang/Object"}}
	parents, known := edges[name]
	return parents, known
}

func phiTypeLeaf(name string) values.JavaValue {
	if name == "" {
		return values.JavaNull
	}
	return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
}

func phiTypeSlot(d *Decompiler, a, b values.JavaValue) *values.SlotValue {
	merge := &OpCode{CurrentOffset: 30}
	left := &OpCode{StackEntry: newStackItem(NewEmptyStackEntry(), a), Target: []*OpCode{merge}}
	right := &OpCode{StackEntry: newStackItem(NewEmptyStackEntry(), b), Target: []*OpCode{merge}}
	merge.Source = []*OpCode{left, right}
	slot := values.NewSlotValue(a, a.Type())
	d.stackPhiSources[slot] = merge
	return slot
}

// This finite oracle enumerates the nominal tree independently: distinct
// Left/Right/Base leaves meet at Base, Other meets them only at Object, and
// null contributes no constraint. Four assembly orders include a shared DAG.
func TestClosedStackPhiTypesMatchFiniteNominalModel(t *testing.T) {
	names := []string{"", "phi.Left", "phi.Right", "phi.Base", "phi.Other"}
	for packed := 0; packed < 625; packed++ {
		code := packed
		var leaves [4]values.JavaValue
		set := map[string]bool{}
		for i := range leaves {
			name := names[code%5]
			code /= 5
			leaves[i] = phiTypeLeaf(name)
			if name != "" {
				set[name] = true
			}
		}
		want := ""
		for name := range set {
			want = name
		}
		if len(set) > 1 {
			want = "phi.Base"
			if set["phi.Other"] {
				want = "java.lang.Object"
			}
		}
		for layout := 0; layout < 4; layout++ {
			d := &Decompiler{stackPhiSources: map[*values.SlotValue]*OpCode{}}
			var root values.JavaValue
			switch layout {
			case 0:
				root = phiTypeSlot(d, phiTypeSlot(d, leaves[0], leaves[1]), phiTypeSlot(d, leaves[2], leaves[3]))
			case 1:
				root = phiTypeSlot(d, phiTypeSlot(d, leaves[3], leaves[2]), phiTypeSlot(d, leaves[1], leaves[0]))
			case 2:
				root = phiTypeSlot(d, leaves[0], phiTypeSlot(d, leaves[1], phiTypeSlot(d, leaves[2], leaves[3])))
			case 3:
				shared := phiTypeSlot(d, leaves[0], leaves[1])
				root = values.NewTernaryExpression(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), phiTypeSlot(d, shared, leaves[2]), phiTypeSlot(d, leaves[3], shared))
				root.(*values.TernaryExpression).SetCachedType(types.NewJavaClass("phi.Poison"))
			}
			got, valid := d.closedStackPhiValueType(root, phiTypeHierarchy)
			name := ""
			if got != nil {
				name, _ = types.RawClassFQN(got)
			}
			if !valid || name != want {
				t.Fatalf("model%d layout%d: got %s/%v want %s", packed, layout, name, valid, want)
			}
			for i, leaf := range leaves {
				if names[packed/intPow5(i)%5] != "" {
					original, _ := types.RawClassFQN(leaf.Type())
					if original != names[packed/intPow5(i)%5] {
						t.Fatal("query changed leaf type")
					}
				}
			}
		}
	}
}

func intPow5(power int) int {
	n := 1
	for i := 0; i < power; i++ {
		n *= 5
	}
	return n
}

func TestClosedStackPhiTypeQueryRejectsIncompleteEvidence(t *testing.T) {
	for _, kind := range []string{"valid", "missing stack", "foreign edge", "missing arm", "cycle", "depth", "nil value", "typed nil", "primitive mismatch", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			d := &Decompiler{stackPhiSources: map[*values.SlotValue]*OpCode{}}
			left, right := phiTypeLeaf("phi.Left"), phiTypeLeaf("phi.Right")
			root := phiTypeSlot(d, left, right)
			var query values.JavaValue = root
			switch kind {
			case "missing stack":
				d.stackPhiSources[root].Source[0].StackEntry = nil
			case "foreign edge":
				d.stackPhiSources[root].Source[0].Target = nil
			case "missing arm":
				d.stackPhiSources[root].Source = d.stackPhiSources[root].Source[:1]
			case "cycle":
				d.stackPhiSources[root].Source[0].StackEntry.value = root
			case "depth":
				for i := 0; i < 130; i++ {
					query = values.NewSlotValue(query, query.Type())
				}
			case "nil value":
				d.stackPhiSources[root].Source[0].StackEntry.value = nil
			case "typed nil":
				d.stackPhiSources[root].Source[0].StackEntry.value = (*values.JavaRef)(nil)
			case "primitive mismatch":
				d.stackPhiSources[root].Source[0].StackEntry.value = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, valid := d.closedStackPhiValueType(query, phiTypeHierarchy)
			if valid != (kind == "valid") {
				t.Fatalf("valid=%v", valid)
			}
			if root.GetValue() != left || len(d.effectfulStackPhiEdges) != 0 {
				t.Fatal("query changed source evidence")
			}
		})
	}
}

func TestClosedStackPhiTypeQueryChargesGrowthWithoutReservingUnusedCapacity(t *testing.T) {
	d := &Decompiler{Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 256})}
	leaf := phiTypeLeaf("phi.Left")
	typ, valid := d.closedStackPhiValueType(leaf, phiTypeHierarchy)
	name, _ := types.RawClassFQN(typ)
	if !valid || name != "phi.Left" {
		t.Fatalf("compact query must fit a 2KiB intermediate budget without reserving all 512 nodes: %s/%v (%v)", name, valid, d.Work.Check())
	}
}

func phiDefinitionDecompiler() (*Decompiler, *values.JavaRef, *values.SlotValue, *OpCode) {
	d := &Decompiler{stackPhiSources: map[*values.SlotValue]*OpCode{}, FunctionContext: &class_context.ClassContext{SiblingSuperTypes: phiTypeHierarchy}}
	seed := phiTypeSlot(d, phiTypeLeaf("phi.Left"), phiTypeLeaf("phi.Right"))
	ref := values.NewJavaRef(utils.NewRootVariableId(), seed, types.NewJavaClass("phi.Left"))
	op := &OpCode{CurrentOffset: 31, Instr: &Instruction{OpCode: OP_DUP}}
	ref.MarkOriginalStackMaterialization(31, OP_DUP, seed)
	d.opcodeIdToRef = map[*OpCode][][2]any{op: {{ref, true}}}
	d.dupConvertedRefValue = map[*OpCode][]values.JavaValue{op: {seed}}
	return d, ref, seed, op
}

func TestClosedStackPhiDefinitionTypesRequireOriginalSingleDefinition(t *testing.T) {
	for _, kind := range []string{"DUP", "local store", "missing witness", "wrong witness PC", "wrong witness kind", "wrong original seed", "reassignment", "same UID other definition", "not first", "parameter", "receiver", "solved web", "custom", "missing original row", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			d, ref, seed, op := phiDefinitionDecompiler()
			switch kind {
			case "local store":
				op.Instr.OpCode = OP_ASTORE_1
				op.stackConsumed = []values.JavaValue{seed}
				d.refToCreatingStore = map[*values.JavaRef]*OpCode{ref: op}
			case "missing witness":
				other := values.NewJavaRef(ref.Id, seed, ref.Type())
				other.VarUid = ref.VarUid
				ref = other
				d.opcodeIdToRef[op][0][0] = ref
			case "wrong witness PC":
				op.CurrentOffset++
			case "wrong witness kind":
				op.Instr.OpCode = OP_NOP
			case "wrong original seed":
				d.dupConvertedRefValue[op] = []values.JavaValue{phiTypeLeaf("phi.Left")}
			case "reassignment":
				d.opcodeIdToRef[&OpCode{CurrentOffset: 32}] = [][2]any{{ref, false}}
			case "same UID other definition":
				other := values.NewJavaRef(utils.NewRootVariableId(), seed, ref.Type())
				other.VarUid = ref.VarUid
				d.opcodeIdToRef[&OpCode{CurrentOffset: 32}] = [][2]any{{other, true}}
			case "not first":
				d.opcodeIdToRef[op][0][1] = false
			case "parameter":
				ref.IsParam = true
			case "receiver":
				ref.IsThis = true
			case "solved web":
				ref.WebDeclType = ref.Type().Copy()
			case "custom":
				ref.CustomValue = &values.CustomValue{}
			case "missing original row":
				d.opcodeIdToRef = nil
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.refineClosedStackPhiDefinitions()
			accepted := kind == "DUP" || kind == "local store"
			name, _ := types.RawClassFQN(ref.Type())
			if accepted {
				if name != "phi.Base" || ref.WebDeclType == nil {
					t.Fatalf("incomplete declaration %s", name)
				}
			} else if name != "phi.Left" || len(d.stackPhiTypes) != 0 {
				t.Fatal("unproved definition changed")
			}
			left, _ := types.RawClassFQN(seed.GetValue().Type())
			if left != "phi.Left" {
				t.Fatal("original arm descriptor changed")
			}
			if accepted {
				tern := values.NewTernaryExpression(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), seed.GetValue(), d.stackPhiSources[seed].Source[1].StackEntry.value)
				d.resetClosedStackPhiValue(seed, tern)
				typ, _ := types.RawClassFQN(tern.Type())
				left, _ := types.RawClassFQN(tern.TrueValue.Type())
				right, _ := types.RawClassFQN(tern.FalseValue.Type())
				if typ != "phi.Base" || left != "phi.Left" || right != "phi.Right" {
					t.Fatal("conditional cache aliases its operands")
				}
			}
		})
	}
}
