package performance_baseline

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func countOpcodes(d *core.Decompiler) WorkCounts {
	w := WorkCounts{}
	seen := map[*core.OpCode]struct{}{}
	var walk func(*core.OpCode)
	walk = func(op *core.OpCode) {
		if op == nil {
			return
		}
		if _, ok := seen[op]; ok {
			return
		}
		seen[op] = struct{}{}
		tallyOp(op, &w)
		for _, n := range op.Target {
			walk(n)
		}
	}
	// ParseOpcode only: Target edges are empty until ScanJmp. Walk the opcode list
	// through a local decode by counting after ParseOpcode via a throwaway ScanJmp
	// would mix stages. Count by walking nothing if no targets: use opcode graph
	// after ScanJmp, or count via a second pass. For decode we recount via
	// a non-exported list, so we ScanJmp-free count using Target if present,
	// else we cannot see opCodes (unexported). After ParseOpcode, RootOpCode is exported.
	walk(d.RootOpCode)
	if w.OpcodeCount == 0 && d.RootOpCode != nil {
		tallyOp(d.RootOpCode, &w)
	}
	return w
}

func countCFG(d *core.Decompiler) WorkCounts {
	w := WorkCounts{}
	seen := map[*core.OpCode]struct{}{}
	handlers := map[*core.OpCode]struct{}{}
	var walk func(*core.OpCode)
	walk = func(op *core.OpCode) {
		if op == nil {
			return
		}
		if _, ok := seen[op]; ok {
			return
		}
		seen[op] = struct{}{}
		tallyOp(op, &w)
		w.CFGNodes++
		w.CFGEdges += len(op.Target)
		if op.IsCatch {
			handlers[op] = struct{}{}
		}
		for _, n := range op.Target {
			walk(n)
		}
	}
	walk(d.RootOpCode)
	w.HandlerPCs = len(handlers)
	w.DefMergeLoads, w.DefStoreLoadPairs = countDefMerges(seen)
	return w
}

func tallyOp(op *core.OpCode, w *WorkCounts) {
	if op == nil || op.Instr == nil {
		return
	}
	w.OpcodeCount++
	code := op.Instr.OpCode
	if code == core.OP_START || code == core.OP_END {
		return
	}
	la := core.LocalAccessOf(code)
	if la.Write && code != core.OP_IINC {
		w.StoreCount++
	}
	if la.Read && code != core.OP_IINC && code != core.OP_RET {
		w.LoadCount++
	}
	if code == core.OP_IINC {
		w.IincCount++
	}
}

func countDefMerges(ops map[*core.OpCode]struct{}) (merges, pairs int) {
	for op := range ops {
		if op == nil || op.Instr == nil {
			continue
		}
		code := op.Instr.OpCode
		la := core.LocalAccessOf(code)
		if !la.Read || code == core.OP_IINC || code == core.OP_RET {
			continue
		}
		slot := core.GetRetrieveIdx(op)
		if slot < 0 {
			continue
		}
		stores := reachingStoresHarness(op, slot)
		pairs += len(stores)
		if len(stores) > 1 {
			merges++
		}
	}
	return
}

// Independent reaching-definition walk over exported Source edges.
// Not a copy of computeSlotWebs; used only as a harness work counter.
func reachingStoresHarness(load *core.OpCode, slot int) []*core.OpCode {
	if load == nil {
		return nil
	}
	visited := map[*core.OpCode]bool{load: true}
	queue := append([]*core.OpCode{}, load.Source...)
	var stores []*core.OpCode
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == nil || visited[cur] {
			continue
		}
		visited[cur] = true
		if cur.Instr != nil && core.LocalAccessOf(cur.Instr.OpCode).Write && cur.Instr.OpCode != core.OP_IINC {
			if core.GetStoreIdx(cur) == slot {
				stores = append(stores, cur)
				continue
			}
		}
		queue = append(queue, cur.Source...)
	}
	return stores
}
