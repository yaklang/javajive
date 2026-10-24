package core

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An adjacent private IINC -> ILOAD edge has no intervening evaluation.
// Preserve its updated result as a prefix operation rather than a separate
// statement: conditional consumers must retain the original evaluation domain.
// All signed IINC operands have the same int-word addition semantics; unit
// increments merely have a shorter source spelling.
// No alternate load entry, handler change, reused binding or post-op occurrence
// grants this permission. The decoder's exact signed operand determines the op.
func (d *Decompiler) immediateLocalPreUpdate(update, load *OpCode, loaded values.JavaValue) (int, bool) {
	// Re-simulating a load must re-prove the same edge and binding; the emission
	// marker is not a reason to turn its prefix operation back into a plain read.
	if d == nil || update == nil || load == nil || update.Instr == nil || load.Instr == nil || update.IsCustom || load.IsCustom || update.IsCatch || load.IsCatch || update.IsTryCatchParent || load.IsTryCatchParent || update.IincFoldedToPostOp || update.Instr.OpCode != OP_IINC || len(update.Target) != 1 || update.Target[0] != load || len(load.Source) != 1 || load.Source[0] != update || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return 0, false
	}
	if load.Instr.OpCode != OP_ILOAD && (load.Instr.OpCode < OP_ILOAD_0 || load.Instr.OpCode > OP_ILOAD_3) {
		return 0, false
	}
	if load.Instr.OpCode == OP_ILOAD {
		width := 1
		if load.IsWide {
			width = 2
		}
		if len(load.Data) != width {
			return 0, false
		}
	} else if load.IsWide || len(load.Data) != 0 {
		return 0, false
	}
	slot, delta, width := 0, 0, 3
	if update.IsWide {
		if len(update.Data) != 4 {
			return 0, false
		}
		slot, delta, width = int(binary.BigEndian.Uint16(update.Data)), int(int16(binary.BigEndian.Uint16(update.Data[2:]))), 6
	} else {
		if len(update.Data) != 2 {
			return 0, false
		}
		slot, delta = int(update.Data[0]), int(int8(update.Data[1]))
	}
	if int(load.CurrentOffset) != int(update.CurrentOffset)+width || GetRetrieveIdx(load) != slot || !sameHandlerCoverage(d.handlersAt(update), d.handlersAt(load)) {
		return 0, false
	}
	// loadVarBySlot supplies one enumerable slot around the resolved reference.
	if slotValue, ok := loaded.(*values.SlotValue); ok && slotValue != nil {
		loaded = slotValue.GetValue()
	}
	ref, ok := loaded.(*values.JavaRef)
	if !ok || ref == nil || ref != update.Ref || ref.IsThis || ref.Type() == nil {
		return 0, false
	}
	primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
	return delta, ok && primitive != nil && primitive.Name == types.JavaInteger
}
