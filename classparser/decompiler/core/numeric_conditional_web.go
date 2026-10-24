package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// ISTORE retains the whole computational int word. A narrow source arm in a
// conditional is not an implicit i2c/i2b/i2s conversion of the conditional's
// result. A complete single-definition web owns its declaration and its loads;
// do not widen a simulator ref shared with an earlier narrow lifetime.
func (d *Decompiler) restoreNumericConditionalWebs() {
	if d == nil || len(d.ExceptionTable) != 0 || len(d.opCodes) > 8192 || d.getenv("JDEC_LIVEINTERVAL_OFF") != "" || !d.hasNumericConditionalStore() {
		return
	}
	webs := d.localUpdateSourceWebs()
	if webs == nil {
		return
	}
	entries := map[int]bool{}
	for _, web := range webs.entryWeb {
		entries[web] = true
	}
	snapshots := map[*utils.VariableId]bool{}
	for _, group := range d.evaluationSnapshots {
		for _, snapshot := range group {
			if snapshot.Operand {
				if ref, ok := dynamicCaptureSeed(snapshot.Value).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
					snapshots[ref.Id] = true
				}
			}
		}
	}
	stores := map[int][]*OpCode{}
	decoded := map[*OpCode]bool{}
	for _, op := range d.opCodes {
		if op != nil {
			decoded[op] = true
		}
	}
	incomplete := map[int]bool{}
	for op, web := range webs.webOf {
		if op == nil || op.Instr == nil {
			incomplete[web] = true
			continue
		}
		if !decoded[op] {
			incomplete[web] = true
		}
		if isLocalStoreOpcode(op.Instr.OpCode) {
			stores[web] = append(stores[web], op)
			if !dynamicPrimitiveStore(op.Instr.OpCode, types.JavaInteger) {
				incomplete[web] = true
			}
		}
		if isLocalLoadOpcode(op.Instr.OpCode) {
			if !dynamicPrimitiveLoad(op.Instr.OpCode, types.JavaInteger) || len(op.stackProduced) != 1 {
				incomplete[web] = true
				continue
			}
			if slot, ok := op.stackProduced[0].(*values.SlotValue); !ok || slot == nil {
				incomplete[web] = true
			}
		}
		if op.Instr.OpCode == OP_IINC {
			incomplete[web] = true
		}
	}
	// Original opcode order makes declaration/identity selection deterministic.
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !dynamicPrimitiveStore(op.Instr.OpCode, types.JavaInteger) {
			continue
		}
		web, known := webs.webOf[op]
		if !known || entries[web] || incomplete[web] || len(stores[web]) != 1 {
			continue
		}
		infos := d.opcodeIdToRef[op]
		if len(infos) != 1 || len(op.stackConsumed) != 1 {
			continue
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.StackVar != nil || ref.CustomValue != nil || ref.WebDeclType != nil || snapshots[ref.Id] || d.localHasBooleanDescriptorUse(ref) {
			continue
		}
		old := ref.Type()
		if old == nil {
			continue
		}
		p, ok := old.RawType().(*types.JavaPrimer)
		if !ok || p == nil || (p.Name != types.JavaByte && p.Name != types.JavaShort && p.Name != types.JavaChar) {
			continue
		}
		value := dynamicCaptureSeed(op.stackConsumed[0])
		conditional, ok := value.(*values.TernaryExpression)
		if !ok || conditional == nil {
			continue
		}
		result, known := values.NumericConditionalType(conditional.TrueValue, conditional.FalseValue)
		if !known || !isExactPrimer(result, types.JavaInteger) {
			continue
		}
		d.bindProvedPrimitiveWeb(webs, web, stores[web], types.JavaInteger)
	}
}

// An original int store consuming a conditional is necessary for every
// rewrite above. Prove applicability before constructing source-update webs.
// This inventory grants no rebinding permission; the complete web, lifetime,
// descriptor and numeric-domain proofs remain mandatory in the main pass.
func (d *Decompiler) hasNumericConditionalStore() bool {
	if d.Work != nil && (d.Work.Check() != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opcodeIdToRef))) != nil) {
		return false
	}
	for op := range d.opcodeIdToRef {
		if op == nil || op.Instr == nil || !dynamicPrimitiveStore(op.Instr.OpCode, types.JavaInteger) || len(op.stackConsumed) != 1 {
			continue
		}
		if conditional, ok := dynamicCaptureSeed(op.stackConsumed[0]).(*values.TernaryExpression); ok && conditional != nil {
			return true
		}
	}
	return false
}
