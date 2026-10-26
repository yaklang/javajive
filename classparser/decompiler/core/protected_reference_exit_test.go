package core

import (
	"context"
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/omap"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestProtectedReferenceExitRequiresClosedOriginalDefinition(t *testing.T) {
	for _, scenario := range []string{"proved", "null seed", "typed null seed", "unshared", "nested interval", "no exit", "unprotected", "missing graph", "failed graph", "missing original store", "missing decoded load", "missing web load", "foreign web store", "multiple definitions", "entry", "missing operand", "missing produced value", "opaque load", "nil forwarding", "wrong slot", "primitive load", "custom load", "parameter", "handler definition", "unknown handler", "malformed interval", "snapshot", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			typ := types.NewJavaClass("java.util.Collection")
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			old.Id.SetName("result")
			handler := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			store, use, reuse, later, entry := op(OP_ASTORE_1, 2), op(OP_ALOAD_1, 5), op(OP_ASTORE_1, 6), op(OP_ALOAD_1, 7), op(OP_ASTORE_2, 8)
			rhs := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			store.stackConsumed = []values.JavaValue{rhs}
			reuse.stackConsumed = []values.JavaValue{rhs}
			entry.stackConsumed = []values.JavaValue{handler}
			own, foreign := values.NewSlotValue(old, typ), values.NewSlotValue(old, typ)
			use.stackProduced = []values.JavaValue{own}
			later.stackProduced = []values.JavaValue{foreign}
			nodes := []*OpCode{store, use, reuse, later, entry}
			webs := &slotWeb{webOf: map[*OpCode]int{store: 1, use: 1, reuse: 2, later: 2, entry: 3}, entryWeb: map[int]int{}}
			d := &Decompiler{semanticCFG: &SemanticCFG{Nodes: append([]*OpCode(nil), nodes...)}, opCodes: nodes, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{store: {{old, true}}, reuse: {{old, false}}, entry: {{handler, true}}}, ExceptionTable: []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}}
			d.varUserMap = omap.NewEmptyOrderedMap[*values.JavaRef, []*VarFoldRule]()
			observed := values.JavaValue(old)
			preserved := values.JavaValue(old)
			d.varUserMap.Set(old, []*VarFoldRule{{CurrentOpcode: use, Replace: func(v values.JavaValue) { observed = v }}, {CurrentOpcode: later, Replace: func(v values.JavaValue) { preserved = v }}})
			switch scenario {
			case "null seed":
				store.stackConsumed[0] = values.JavaNull
			case "typed null seed":
				store.stackConsumed[0] = values.NewJavaLiteral("null", typ)
			case "unshared":
				d.opcodeIdToRef[reuse][0][0] = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "nested interval":
				d.ExceptionTable = append(d.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: 8, HandlerPc: 8})
			case "no exit":
				d.ExceptionTable[0].EndPc = 6
			case "unprotected":
				d.ExceptionTable[0].StartPc = 3
			case "missing graph":
				d.semanticCFG = nil
			case "failed graph":
				d.semanticCFG.Err = errors.New("incomplete")
			case "missing original store":
				d.semanticCFG.Nodes = d.semanticCFG.Nodes[1:]
			case "missing decoded load":
				d.opCodes = []*OpCode{store, reuse, later, entry}
			case "missing web load":
				delete(webs.webOf, use)
			case "foreign web store":
				webs.webOf[op(OP_ASTORE_1, 10)] = 1
			case "multiple definitions":
				webs.webOf[reuse] = 1
			case "entry":
				webs.entryWeb[1] = 1
			case "missing operand":
				store.stackConsumed = nil
			case "missing produced value":
				use.stackProduced = nil
			case "opaque load":
				use.stackProduced = []values.JavaValue{old}
			case "nil forwarding":
				use.stackProduced = []values.JavaValue{values.NewSlotValue(nil, typ)}
			case "wrong slot":
				use.Instr = op(OP_ALOAD_2, 5).Instr
			case "primitive load":
				use.Instr = op(OP_ILOAD_1, 5).Instr
			case "custom load":
				use.IsCustom = true
			case "parameter":
				old.IsParam = true
			case "handler definition":
				d.ExceptionTable[0].HandlerPc = 2
			case "unknown handler":
				d.ExceptionTable[0].HandlerPc = 99
			case "malformed interval":
				d.ExceptionTable[0].EndPc = 0
			case "snapshot":
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{use: {{Ref: old, Value: old}}}
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			}
			d.partitionSharedReferenceWebs()
			got := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
			want := scenario == "proved" || scenario == "unshared" || scenario == "nested interval"
			if (got != old) != want {
				t.Fatalf("isolated=%v want=%v", got != old, want)
			}
			if want {
				got.MarkOriginalLocalDeclaration(int(store.CurrentOffset), GetStoreIdx(store), rhs)
				pc, slot, sealed := got.OriginalLocalDeclaration(rhs)
				if !sealed || pc != int(store.CurrentOffset) || slot != GetStoreIdx(store) {
					t.Fatal("lost original single-definition receipt")
				}
			}
			if want && (got.Id == old.Id || got.SolvedWebIdentity != got.Id || own.GetValue() != got || observed != got || foreign.GetValue() != old || preserved != old || store.stackConsumed[0] != rhs || got.Val != rhs) {
				t.Fatal("changed foreign lifetime/producer, lost owned use, or invented initializer")
			}
		})
	}
}
