package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A LOAD reached only by method entry reads the descriptor-seeded parameter,
// even when DFS has already simulated a later same-slot STORE on another arm.
// No source type/name comparison can distinguish those lifetimes. Consult the
// immutable CFG before registering the load operand and its folding callback;
// mixed entry/store joins still belong to the ordinary web solver.
func (d *Decompiler) originalParameterLoadBinding(load *OpCode, slot int) *values.JavaRef {
	if d == nil || d.getenv("JDEC_LIVEINTERVAL_OFF") != "" || load == nil || load.Instr == nil || load.IsCustom || !isReferenceLoadOpcode(load.Instr.OpCode) || GetRetrieveIdx(load) != slot || d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.semanticCFG.Nodes) > 8192 {
		return nil
	}
	width := 0
	if load.Instr.OpCode == OP_ALOAD {
		width = 1
		if load.IsWide {
			width = 2
		}
	} else if load.IsWide {
		return nil
	}
	if len(load.Data) != width || len(d.Params) > 256 {
		return nil
	}
	g := d.semanticCFG
	if _, known := g.order[load]; !known {
		return nil
	}
	if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.Params)+1)) != nil {
		return nil
	}
	var parameter *values.JavaRef
	for _, value := range d.Params {
		ref, ok := value.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.Type() == nil || refIsPrimitive(ref) {
			continue
		}
		originalSlot, known := ref.OriginalParameterSlot()
		if !known {
			originalSlot, known = ref.OriginalReceiverSlot()
		}
		if known && originalSlot == slot {
			if parameter != nil {
				return nil
			}
			parameter = ref
		}
	}
	if parameter == nil {
		return nil
	}
	definitions, entry := g.ReachingDefinitions(load, slot)
	if g.Err != nil || !entry || len(definitions) != 0 {
		return nil
	}
	return parameter
}
