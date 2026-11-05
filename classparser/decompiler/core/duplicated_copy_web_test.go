package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDuplicatedPrimitiveCopyKeepsDistinctLocalLifetime(t *testing.T) {
	for _, variant := range []string{"int", "byte", "short", "char", "char declaration", "long", "float", "double", "boolean", "missing witness", "changed seed", "wrong pc", "wrong kind", "missing conversion", "wrong ref", "missing producer", "forged operand", "partial duplicate", "mixed incoming", "exception edge", "foreign successor", "missing original", "malformed duplicate", "malformed store", "mutable graph", "category two mixed cell"} {
		t.Run(variant, func(t *testing.T) {
			kind, sourceKind, duplicateCode, storeCode, loadCode := types.JavaInteger, types.JavaInteger, OP_DUP, OP_ISTORE_1, OP_ILOAD_1
			switch variant {
			case "byte", "short", "char", "boolean":
				sourceKind = variant
			case "char declaration":
				kind, sourceKind = types.JavaChar, types.JavaChar
			case "long", "category two mixed cell":
				kind, sourceKind, duplicateCode, storeCode, loadCode = types.JavaLong, types.JavaLong, OP_DUP2, OP_LSTORE_1, OP_LLOAD_1
			case "float":
				kind, sourceKind, storeCode, loadCode = types.JavaFloat, types.JavaFloat, OP_FSTORE_1, OP_FLOAD_1
			case "double":
				kind, sourceKind, duplicateCode, storeCode, loadCode = types.JavaDouble, types.JavaDouble, OP_DUP2, OP_DSTORE_1, OP_DLOAD_1
			}
			typ, sourceType := types.NewJavaPrimer(kind), types.NewJavaPrimer(sourceKind)
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			old.Id.SetName("reused")
			word := values.NewJavaLiteral(-37, sourceType)
			tmp := values.NewJavaRef(utils.NewRootVariableId(), word, sourceType)
			tmp.MarkOriginalStackMaterialization(4, duplicateCode, word)
			input, output := values.NewSlotValue(tmp, sourceType), values.NewSlotValue(old, typ)
			a, duplicate, b, use, other := op(storeCode, 1), op(duplicateCode, 4), op(storeCode, 5), op(loadCode, 6), op(OP_ISTORE_3, 7)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(73, typ)}
			duplicate.stackConsumed, duplicate.stackProduced = []values.JavaValue{word}, []values.JavaValue{input, input}
			if duplicateCode == OP_DUP2 {
				duplicate.stackConsumed = append(duplicate.stackConsumed, input)
				duplicate.stackProduced = append(duplicate.stackProduced, input)
			}
			if variant == "char declaration" {
				a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(73, types.NewJavaPrimer(types.JavaInteger))}
			}
			b.stackConsumed, use.stackProduced = []values.JavaValue{input}, []values.JavaValue{output}
			other.stackConsumed = []values.JavaValue{values.NewJavaLiteral(17, types.NewJavaPrimer(types.JavaInteger))}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 2, use: 2, other: 3}, entryWeb: map[int]int{}}
			g := &SemanticCFG{Nodes: []*OpCode{a, duplicate, b, use, other}, order: map[*OpCode]int{a: 0, duplicate: 1, b: 2, use: 3, other: 4}, Edges: []SemanticEdge{{From: duplicate, To: b, Kind: EdgeFallthrough}}, incoming: map[*OpCode][]int{b: {0}}, outgoing: map[*OpCode][]int{duplicate: {0}}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, semanticCFG: g, cachedSlotWebs: webs, opCodes: g.Nodes, opcodeIdToRef: map[*OpCode][][2]any{a: {{old, true}}, b: {{old, false}}, duplicate: {{tmp, true}}, other: {{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger)), true}}}, dupConvertedRefValue: map[*OpCode][]values.JavaValue{duplicate: {word}}, stackValueProducers: map[values.JavaValue]*OpCode{input: duplicate}}
			want := variant == "int" || variant == "byte" || variant == "short" || variant == "char" || variant == "char declaration" || variant == "long" || variant == "float" || variant == "double" || variant == "mutable graph"
			switch variant {
			case "missing witness":
				tmp = values.NewJavaRef(tmp.Id, word, sourceType)
				input.ResetValue(tmp)
				d.opcodeIdToRef[duplicate][0][0] = tmp
			case "changed seed":
				tmp.Val = values.NewJavaLiteral(-37, sourceType)
			case "wrong pc":
				duplicate.CurrentOffset = 3
			case "wrong kind":
				duplicate.Instr = op(OP_DUP_X1, 4).Instr
			case "missing conversion":
				d.dupConvertedRefValue[duplicate] = nil
			case "wrong ref":
				d.opcodeIdToRef[duplicate][0][0] = old
			case "missing producer":
				delete(d.stackValueProducers, input)
			case "forged operand":
				b.stackConsumed[0] = values.NewSlotValue(tmp, sourceType)
			case "partial duplicate":
				duplicate.stackProduced = duplicate.stackProduced[:1]
			case "mixed incoming":
				g.incoming[b] = []int{0, 0}
			case "exception edge":
				g.Edges[0].Kind = EdgeException
			case "foreign successor":
				g.outgoing[duplicate] = []int{0, 0}
			case "missing original":
				delete(g.order, duplicate)
			case "malformed duplicate":
				duplicate.Data = []byte{1}
			case "malformed store":
				b.Data = []byte{1}
			case "mutable graph":
				b.Source, duplicate.Target = []*OpCode{other}, []*OpCode{other}
			case "category two mixed cell":
				duplicate.stackConsumed[1] = values.NewSlotValue(tmp, sourceType)
			}
			originalDuplicate := d.opcodeIdToRef[duplicate][0][0]
			d.partitionSharedPrimitiveCopies()
			actual := d.opcodeIdToRef[b][0][0].(*values.JavaRef)
			if (actual != old) != want {
				t.Fatalf("partitioned=%v want=%v", actual != old, want)
			}
			if d.opcodeIdToRef[a][0][0] != old || d.opcodeIdToRef[duplicate][0][0] != originalDuplicate || b.stackConsumed[0] == nil {
				t.Fatal("changed the other lifetime or original stack evaluation")
			}
			if want && (values.UnpackSoltValue(output) != actual || actual.SolvedWebIdentity == nil || !isExactPrimer(actual.Type(), kind) || b.stackConsumed[0] != input) {
				t.Fatal("lost stored word, complete load ownership, or declaration identity")
			}
		})
	}
}
