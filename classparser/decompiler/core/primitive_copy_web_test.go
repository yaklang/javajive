package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestPrimitiveCopyPartitionRequiresCompleteIndependentLifetime(t *testing.T) {
	for _, scenario := range []string{"int", "long", "float", "double", "consumer view", "excessive views", "narrow", "boolean", "entry", "missing producer", "ambiguous producer", "missing source web", "missing store", "missing use", "wrong use kind", "same slot", "same web", "no shared lifetime", "handler", "dynamic snapshot", "increment", "increment in another web", "source domain", "work cap", "allocation cap", "second definition"} {
		t.Run(scenario, func(t *testing.T) {
			kind, storeCode, loadCode, sourceCode := types.JavaInteger, OP_ISTORE_1, OP_ILOAD_1, OP_ILOAD_3
			switch scenario {
			case "long":
				kind, storeCode, loadCode, sourceCode = types.JavaLong, OP_LSTORE_1, OP_LLOAD_1, OP_LLOAD_3
			case "float":
				kind, storeCode, loadCode, sourceCode = types.JavaFloat, OP_FSTORE_1, OP_FLOAD_1, OP_FLOAD_3
			case "double":
				kind, storeCode, loadCode, sourceCode = types.JavaDouble, OP_DSTORE_1, OP_DLOAD_1, OP_DLOAD_3
			case "narrow":
				kind = types.JavaByte
			case "boolean":
				kind = types.JavaBoolean
			}
			typ := types.NewJavaPrimer(kind)
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			old.Id.SetName("word")
			source := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			a, load, b, use := op(storeCode, 1), op(sourceCode, 2), op(storeCode, 3), op(loadCode, 4)
			input, output := values.NewSlotValue(source, typ), values.NewSlotValue(old, typ)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(3, typ)}
			load.stackProduced = []values.JavaValue{input}
			b.stackConsumed = []values.JavaValue{input}
			use.stackProduced = []values.JavaValue{output}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, load: 2, b: 3, use: 3}, entryWeb: map[int]int{3: 2}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, cachedSlotWebs: webs, opCodes: []*OpCode{a, load, b, use}, opcodeIdToRef: map[*OpCode][][2]any{a: {{old, true}}, b: {{old, false}}}}
			switch scenario {
			case "consumer view", "excessive views":
				depth := 1
				if scenario == "excessive views" {
					depth = 17
				}
				for i := 0; i < depth; i++ {
					b.stackConsumed[0] = values.NewSlotValue(b.stackConsumed[0], typ)
				}
			case "entry":
				webs.entryWeb[1] = 3
			case "missing producer":
				load.stackProduced = nil
			case "ambiguous producer":
				duplicate := op(sourceCode, 7)
				duplicate.stackProduced = []values.JavaValue{input}
				d.opCodes = append(d.opCodes, duplicate)
				webs.webOf[duplicate] = 2
			case "missing source web":
				delete(webs.webOf, load)
			case "missing store":
				webs.webOf[op(storeCode, 8)] = 3
			case "missing use":
				use.stackProduced = nil
			case "wrong use kind":
				use.Instr = op(OP_ALOAD_1, 4).Instr
			case "same slot":
				load.Instr = op(loadCode, 2).Instr
			case "same web":
				webs.webOf[load] = 3
			case "no shared lifetime":
				d.opcodeIdToRef[a][0][0] = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 5}}
			case "dynamic snapshot":
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{op(OP_INVOKEDYNAMIC, 5): {{Value: old, Operand: true}}}
			case "increment", "increment in another web":
				inc := op(OP_IINC, 6)
				inc.Data = []byte{1, 1}
				d.opCodes = append(d.opCodes, inc)
				webs.webOf[inc] = 3
				if scenario == "increment in another web" {
					webs.webOf[inc] = 4
				}
			case "source domain":
				source.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "work cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "second definition":
				webs.webOf[a] = 3
			}
			d.partitionSharedPrimitiveCopies()
			got := d.opcodeIdToRef[b][0][0].(*values.JavaRef)
			want := scenario == "int" || scenario == "long" || scenario == "float" || scenario == "double" || scenario == "consumer view"
			if (got != old) != want {
				t.Fatalf("partitioned=%v want=%v", got != old, want)
			}
			if want {
				if got.SolvedWebIdentity != got.Id || got.Val != nil || output.GetValue() != got || input.GetValue() != source || d.opcodeIdToRef[a][0][0] != old || !isExactPrimer(got.Type(), kind) {
					t.Fatal("snapshot lost its own loads or changed another lifetime")
				}
				d.partitionSharedPrimitiveCopies()
				if d.opcodeIdToRef[b][0][0] != got {
					t.Fatal("partition is not idempotent")
				}
			}
		})
	}
}
