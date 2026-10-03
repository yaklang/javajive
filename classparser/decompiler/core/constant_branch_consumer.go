package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Project a pure local read to a constant only at its exact IFEQ/IFNE consumer.
// Original reaching definitions (including exceptional before-state) must all
// be the same literal store. Never use the simulator's last Val, alter a shared
// reference, remove an evaluation, or modify the immutable CFG/web partition.
func (d *Decompiler) restoreConstantBranchConsumers() {
	g := d.semanticCFG
	if g == nil || g.Err != nil {
		return
	}
	remaining := 512
	for _, branch := range g.Nodes {
		if branch == nil || branch.Instr == nil || (branch.Instr.OpCode != OP_IFEQ && branch.Instr.OpCode != OP_IFNE) || len(branch.stackConsumed) != 1 {
			continue
		}
		remaining--
		if remaining < 0 || !d.seedGraphStep(g) {
			return
		}
		edges := g.incoming[branch]
		if len(edges) != 1 {
			continue
		}
		edge := g.Edges[edges[0]]
		load := edge.From
		if edge.Kind != EdgeFallthrough || load == nil || load.Instr == nil || !isLocalLoadOpcode(load.Instr.OpCode) || LocalAccessOf(load.Instr.OpCode).Width != 1 || !integerBranchLoadOpcode(load.Instr.OpCode) || len(load.stackProduced) != 1 {
			continue
		}
		if g.order[load]+1 != g.order[branch] {
			continue
		}
		ref, ok := constantBranchOperand(branch.stackConsumed[0]).(*values.JavaRef)
		if !ok || ref == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref != constantBranchOperand(load.stackProduced[0]) {
			continue
		}
		defs, entry := g.ReachingDefinitions(load, GetRetrieveIdx(load))
		if g.Err != nil {
			return
		}
		if entry || len(defs) == 0 || len(defs) > 32 {
			continue
		}
		known, word := true, -1
		for _, store := range defs {
			remaining--
			if remaining < 0 || !d.seedGraphStep(g) {
				return
			}
			if store == nil || store.Instr == nil || !isLocalStoreOpcode(store.Instr.OpCode) || !integerBranchStoreOpcode(store.Instr.OpCode) || GetStoreIdx(store) != GetRetrieveIdx(load) {
				known = false
				break
			}
			incoming := g.incoming[store]
			if len(incoming) != 1 {
				known = false
				break
			}
			e := g.Edges[incoming[0]]
			constant := e.From
			if e.Kind != EdgeFallthrough || constant == nil || constant.Instr == nil || (constant.Instr.OpCode != OP_ICONST_0 && constant.Instr.OpCode != OP_ICONST_1) {
				known = false
				break
			}
			if g.order[constant]+1 != g.order[store] || constant.IsWide || len(constant.Data) != 0 {
				known = false
				break
			}
			v := constant.Instr.OpCode - OP_ICONST_0
			if word != -1 && v != word {
				known = false
				break
			}
			word = v
		}
		if known && word != -1 {
			branch.stackConsumed[0] = values.NewJavaLiteral(word != 0, types.NewJavaPrimer(types.JavaBoolean))
		}
	}
}

func integerBranchLoadOpcode(op int) bool {
	return op == OP_ILOAD || op >= OP_ILOAD_0 && op <= OP_ILOAD_3
}
func integerBranchStoreOpcode(op int) bool {
	return op == OP_ISTORE || op >= OP_ISTORE_0 && op <= OP_ISTORE_3
}

// Bounded slot unwrapping deliberately does not inspect a JavaRef's last Val.
func constantBranchOperand(v values.JavaValue) values.JavaValue {
	for i := 0; i < 32; i++ {
		s, ok := v.(*values.SlotValue)
		if !ok {
			return v
		}
		if s == nil {
			return nil
		}
		v = s.GetValue()
	}
	return nil
}
