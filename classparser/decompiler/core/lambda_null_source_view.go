package core

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The null singleton is assignable to every reference domain. Recover a typed
// eager copy only from original LOAD/STORE producers and the complete local
// reaching-definition frontier, including exceptional input-state edges.
// ref.Val is store history, never a reaching-value fact.
func (d *Decompiler) refineNullLambdaCaptureOperands() {
	if d == nil || d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.semanticCFG.Nodes) > 65536 || d.FunctionContext == nil {
		return
	}
	memo := map[*OpCode]uint8{}
	var null func(*OpCode, int) bool
	null = func(op *OpCode, depth int) bool {
		if depth > 64 || op == nil || !d.nullCaptureOriginalInstruction(op) || d.semanticCFG.indexOfNode(op) < 0 || len(memo) > 4096 || d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || d.Work.CheckAlloc(int64(len(memo)+1)*32) != nil) {
			return false
		}
		if prior := memo[op]; prior != 0 {
			return prior == 2
		}
		memo[op] = 1
		proved := false
		defer func() {
			if proved {
				memo[op] = 2
			} else {
				memo[op] = 3
			}
		}()
		if op.Instr.OpCode == OP_ACONST_NULL {
			proved = true
			return true
		}
		if !isReferenceLoadOpcode(op.Instr.OpCode) {
			return false
		}
		slot := GetRetrieveIdx(op)
		if slot < 0 {
			return false
		}
		defs, entry := d.semanticCFG.ReachingDefinitions(op, slot)
		if entry || len(defs) == 0 || len(defs) > 1024 || d.semanticCFG.Err != nil || d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(defs))) != nil || d.Work.CheckAlloc(int64(len(defs))*16) != nil) {
			return false
		}
		for _, store := range defs {
			if store == nil || !d.nullCaptureOriginalInstruction(store) || !isReferenceStoreOpcode(store.Instr.OpCode) || GetStoreIdx(store) != slot || len(store.stackConsumed) != 1 {
				return false
			}
			if !null(d.stackValueProducers[store.stackConsumed[0]], depth+1) {
				return false
			}
		}
		proved = true
		return true
	}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC || !d.nullCaptureOriginalInstruction(op) || len(d.evaluationSnapshots[op]) > 64 {
			continue
		}
		for _, snapshot := range d.evaluationSnapshots[op] {
			if !snapshot.Operand || !snapshot.lambdaCapture || snapshot.Ref == nil || snapshot.ExpectedType == nil || snapshot.OriginPC != int(op.CurrentOffset) {
				continue
			}
			pc, index, pending := snapshot.Ref.OriginalPendingDynamicOperand(snapshot.Value)
			if !pending || pc != snapshot.OriginPC {
				continue
			}
			source := dynamicCaptureSeed(snapshot.Value)
			if source == nil || source.Type() == nil {
				continue
			}
			actual, typed := values.SourceTypeErasure(source.Type(), d.FunctionContext)
			expected, known := values.SourceTypeErasure(snapshot.ExpectedType, d.FunctionContext)
			if !typed || !known || !callbinding.Reference(actual) || !callbinding.Reference(expected) {
				continue
			}
			producer := d.stackValueProducers[snapshot.Value]
			if producer == nil || producer.CurrentOffset >= op.CurrentOffset || !null(producer, 0) {
				continue
			}
			if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(actual)+len(expected))+1) != nil || d.Work.CheckAlloc(int64(len(actual)+len(expected))*8+512) != nil) {
				continue
			}
			snapshot.Ref.MarkOriginalNullDynamicOperandView(pc, index, snapshot.Value, snapshot.ExpectedType)
		}
	}
}

// Reopen the exact narrow opcode packet. Mutated PC/opcode/data, WIDE nodes,
// invented graph nodes and a source-only null initializer confer no permission.
func (d *Decompiler) nullCaptureOriginalInstruction(op *OpCode) bool {
	if d == nil || op == nil || op.IsWide || op.IsCustom || op.Instr == nil {
		return false
	}
	n := -1
	switch k := op.Instr.OpCode; {
	case k == OP_ACONST_NULL || k >= OP_ALOAD_0 && k <= OP_ALOAD_3 || k >= OP_ASTORE_0 && k <= OP_ASTORE_3:
		n = 0
	case k == OP_ALOAD || k == OP_ASTORE:
		n = 1
	case k == OP_INVOKEDYNAMIC:
		n = 4
	}
	pc := int(op.CurrentOffset)
	return n >= 0 && len(op.Data) == n && pc >= 0 && pc+1+n <= len(d.bytecodes) && int(d.bytecodes[pc]) == op.Instr.OpCode && bytes.Equal(d.bytecodes[pc+1:pc+1+n], op.Data)
}

// A source capture's owned null view still retains its real LOAD and private
// seed. The older ordinary-web optimization must not erase that binder while
// propagating other null-only locals elsewhere in the same method.
func (d *Decompiler) nullCaptureSourceLoads() map[*OpCode]bool {
	result := map[*OpCode]bool{}
	for _, snapshots := range d.evaluationSnapshots {
		for _, snapshot := range snapshots {
			if snapshot.Ref != nil {
				if _, _, known := snapshot.Ref.OriginalDynamicOperandSourceView(snapshot.Value); known {
					if producer := d.stackValueProducers[snapshot.Value]; producer != nil {
						result[producer] = true
					}
				}
			}
		}
	}
	return result
}
