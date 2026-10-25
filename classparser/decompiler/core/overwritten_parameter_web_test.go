package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestOverwrittenParameterWebKeepsEntryReadsAndSignatureBinder(t *testing.T) {
	for _, name := range []string{"proved", "entry survives", "unknown parameter", "wrong physical slot", "missing store", "missing load", "undecoded load", "wrong load", "increment", "no reads", "handler", "capture", "work cap", "allocation cap"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaClass("java.util.Map")
			param := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			param.IsParam = true
			param.Id.SetName("input")
			other := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.LinkedHashMap"))
			other.Id.SetName("fresh")
			entry, a, b, load := op(OP_ALOAD_0, 1), op(OP_ASTORE_0, 2), op(OP_ASTORE_0, 3), op(OP_ALOAD_0, 4)
			before, after := values.NewSlotValue(param, typ), values.NewSlotValue(other, other.Type())
			entry.stackProduced = []values.JavaValue{before}
			load.stackProduced = []values.JavaValue{after}
			a.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, typ)}
			b.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, other.Type())}
			webs := &slotWeb{webOf: map[*OpCode]int{entry: 0, a: 1, b: 1, load: 1}, entryWeb: map[int]int{0: 0}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, Params: []values.JavaValue{param}, cachedSlotWebs: webs, opCodes: []*OpCode{entry, a, b, load}, opcodeIdToRef: map[*OpCode][][2]any{a: {{param, false}}, b: {{other, true}}}}
			switch name {
			case "entry survives":
				webs.entryWeb[0] = 1
			case "unknown parameter":
				d.Params = nil
			case "wrong physical slot":
				a.Instr = op(OP_ASTORE_1, 2).Instr
			case "missing store":
				webs.webOf[op(OP_ASTORE_0, 9)] = 1
			case "missing load":
				load.stackProduced = nil
			case "undecoded load":
				d.opCodes = []*OpCode{entry, a, b}
			case "wrong load":
				load.Instr = op(OP_ILOAD_0, 4).Instr
			case "increment":
				inc := op(OP_IINC, 6)
				inc.Data = []byte{0, 1}
				d.opCodes = append(d.opCodes, inc)
				webs.webOf[inc] = 1
			case "no reads":
				delete(webs.webOf, load)
			case "handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 5}}
			case "capture":
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{op(OP_INVOKEDYNAMIC, 6): {{Value: param, Operand: true}}}
			case "work cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			d.partitionSharedReferenceWebs()
			got := d.opcodeIdToRef[a][0][0].(*values.JavaRef)
			want := name == "proved"
			if (got != param) != want {
				t.Fatalf("partitioned=%v want=%v", got != param, want)
			}
			if before.GetValue() != param || !param.IsParam || param.Id.String() != "input" || param.Type().String(d.FunctionContext) != "Map" {
				t.Fatal("changed entry parameter")
			}
			if want && (got.IsParam || got.Id == param.Id || got.SolvedWebIdentity != got.Id || got.WebDeclType == nil || after.GetValue() != got || d.opcodeIdToRef[b][0][0] != got) {
				t.Fatal("new lifetime not consistently bound")
			}
		})
	}
}
