package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestUnreadReferenceDefinitionDoesNotOwnParameterBackedge(t *testing.T) {
	for _, kind := range []string{"unread", "effectful", "read", "unknown load", "unknown store", "extra member", "entry", "handler", "capture", "parameter alias", "unshared", "missing value", "original unknown load", "original missing store", "increment", "return address", "invalid store slot", "invalid load slot", "instruction cap", "work cap", "allocation cap"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			param := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			param.IsParam = true
			param.Id.SetName("current")
			shared := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			entry, dead, write, read := op(OP_ALOAD_0, 0), op(OP_ASTORE_0, 2), op(OP_ASTORE_0, 5), op(OP_ALOAD_0, 6)
			before, after := values.NewSlotValue(param, typ), values.NewSlotValue(shared, typ)
			entry.stackProduced = []values.JavaValue{before}
			read.stackProduced = []values.JavaValue{after}
			dead.stackConsumed = []values.JavaValue{values.JavaNull}
			rhs := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			write.stackConsumed = []values.JavaValue{rhs}
			webs := &slotWeb{webOf: map[*OpCode]int{entry: 10, dead: 20, write: 10, read: 10}, entryWeb: map[int]int{0: 10}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, Params: []values.JavaValue{param}, cachedSlotWebs: webs, opCodes: []*OpCode{entry, dead, write, read}, opcodeIdToRef: map[*OpCode][][2]any{dead: {{shared, true}}, write: {{shared, false}}}}
			switch kind {
			case "effectful":
				dead.stackConsumed = []values.JavaValue{&values.FunctionCallExpression{IsStatic: true, ClassName: "example.Producer", FunctionName: "read", Descriptor: "()Ljava/lang/Object;", FuncType: &types.JavaFuncType{ReturnType: typ}}}
			case "read", "unknown load":
				load := op(OP_ALOAD_0, 3)
				load.stackProduced = []values.JavaValue{values.NewSlotValue(shared, typ)}
				d.opCodes = append(d.opCodes, load)
				if kind == "read" {
					webs.webOf[load] = 20
				}
			case "unknown store":
				d.opCodes = append(d.opCodes, op(OP_ASTORE_0, 8))
			case "extra member":
				webs.webOf[op(OP_ALOAD_0, 3)] = 20
			case "entry":
				webs.entryWeb[0] = 20
			case "handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 5, HandlerPc: 6}}
			case "capture":
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{op(OP_INVOKEDYNAMIC, 7): {{Value: shared, Operand: true}}}
			case "parameter alias":
				shared.IsParam = true
			case "unshared":
				d.opcodeIdToRef[write][0][0] = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "missing value":
				dead.stackConsumed = nil
			case "original unknown load":
				d.semanticCFG = &SemanticCFG{Nodes: append(append([]*OpCode{}, d.opCodes...), op(OP_ALOAD_0, 3))}
			case "original missing store":
				d.semanticCFG = &SemanticCFG{Nodes: []*OpCode{entry, write, read}}
			case "increment", "return address":
				code := OP_IINC
				if kind == "return address" {
					code = OP_RET
				}
				op := op(code, 3)
				op.Data = []byte{0, 1}
				d.opCodes = append(d.opCodes, op)
			case "invalid store slot":
				dead.Instr = op(OP_ASTORE, 2).Instr
			case "invalid load slot":
				d.opCodes = append(d.opCodes, op(OP_ALOAD, 3))
			case "instruction cap":
				d.semanticCFG = &SemanticCFG{Nodes: make([]*OpCode, 8193)}
			case "work cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			var deadRHS values.JavaValue
			if len(dead.stackConsumed) == 1 {
				deadRHS = dead.stackConsumed[0]
			}
			d.partitionSharedReferenceWebs()
			isolated := d.opcodeIdToRef[dead][0][0].(*values.JavaRef)
			proved := kind == "unread" || kind == "effectful"
			if (isolated != shared) != proved {
				t.Fatalf("isolated unread definition=%v", isolated != shared)
			}
			if !proved {
				return
			}
			if dead.stackConsumed[0] != deadRHS || write.stackConsumed[0] != rhs || isolated.Id == shared.Id || isolated.Val != nil {
				t.Fatal("changed RHS effects or retained a foreign binding")
			}
			d.unifyReferenceWebs()
			bound := d.opcodeIdToRef[write][0][0].(*values.JavaRef)
			if bound.Id != param.Id || after.GetValue().(*values.JavaRef).Id != param.Id || before.GetValue() != param || isolated.Id == param.Id {
				t.Fatal("entry and backedge reads/writes lost the shared parameter binder")
			}
			if d.opcodeIdToRef[write][0][1] != false {
				t.Fatal("parameter assignment became a new local declaration")
			}
			if !bound.IsParam {
				t.Fatal("parameter alias lost declaration ownership")
			}
			d.unifyReferenceWebs()
			if bound.Id != param.Id || bound.VarUid != param.VarUid || !bound.IsParam || isolated.IsParam {
				t.Fatal("parameter identity is not stable across solver iterations")
			}
		})
	}
}

func TestReferenceParameterAliasRequiresCompleteIdentity(t *testing.T) {
	for _, kind := range []string{"same declaration", "fresh local", "different parameter", "same id only", "same uid only", "missing identity", "foreign web"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			param := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			param.IsParam = true
			alias := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			alias.IsParam = true
			store := op(OP_ASTORE_0, 1)
			store.stackConsumed = []values.JavaValue{values.JavaNull}
			webs := &slotWeb{webOf: map[*OpCode]int{store: 1}, entryWeb: map[int]int{0: 1}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, Params: []values.JavaValue{param}, opCodes: []*OpCode{store}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{store: {{alias, true}}}}
			switch kind {
			case "same declaration", "foreign web":
				alias.Id, alias.VarUid = param.Id, param.VarUid
				if kind == "foreign web" {
					foreign := op(OP_ASTORE_1, 2)
					foreign.stackConsumed = []values.JavaValue{values.JavaNull}
					d.opCodes = append(d.opCodes, foreign)
					webs.webOf[foreign] = 2
					d.opcodeIdToRef[foreign] = [][2]any{{alias, true}}
				}
			case "fresh local":
				alias.IsParam = false
			case "same id only":
				alias.Id = param.Id
			case "same uid only":
				alias.VarUid = param.VarUid
			case "missing identity":
				alias.Id, param.Id, alias.VarUid, param.VarUid = nil, nil, "", ""
			}
			d.unifyReferenceWebs()
			proved := kind == "same declaration" || kind == "fresh local"
			if (alias.WebDeclType != nil) != proved {
				t.Fatalf("parameter declaration solved=%v", alias.WebDeclType != nil)
			}
			if proved && (!alias.IsParam || alias.Id != param.Id || alias.VarUid != param.VarUid || d.opcodeIdToRef[store][0][1] != false) {
				t.Fatal("parameter declaration and assignment identity disagree")
			}
		})
	}
}
