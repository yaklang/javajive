package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A descriptor seed is available at a particular LOAD if no path from method
// entry can overwrite one of its words before that LOAD. This is a two-state
// product of the original semantic CFG and an irreversible clobbered bit, not
// a physical-prefix scan. Later stores are harmless only when they cannot flow
// back to the read. Exception edges use the throwing instruction's input state.
type nativeEnumParameterFlow struct {
	entry *core.OpCode
	byPC  map[int]*core.OpCode
	edges map[*core.OpCode][]core.SemanticEdge
	code  *CodeAttribute
}

func nativeEnumParameterOriginalFlow(d *core.Decompiler, code *CodeAttribute, work *workbudget.Budget) *nativeEnumParameterFlow {
	if d == nil || code == nil || len(code.Code) == 0 || len(code.Code) > 65535 || len(code.ExceptionTable) > 8192 || !nativeProofWork(work, int64(len(code.ExceptionTable))+1) || work != nil && work.CheckAlloc(int64(len(code.ExceptionTable))*64) != nil {
		return nil
	}
	d.ExceptionTable = nil
	for _, handler := range code.ExceptionTable {
		if handler == nil {
			return nil
		}
		d.ExceptionTable = append(d.ExceptionTable, &core.ExceptionTableEntry{StartPc: handler.StartPc, EndPc: handler.EndPc, HandlerPc: handler.HandlerPc, CatchType: handler.CatchType})
	}
	g, err := d.BuildSemanticCFG()
	if err != nil || g == nil || g.Err != nil || len(g.Nodes) == 0 || len(g.Nodes) > 8192 || !nativeProofWork(work, int64(len(g.Nodes)+len(g.Edges))) || work != nil && work.CheckAlloc(int64(len(g.Nodes))*128+int64(len(g.Edges))*64) != nil {
		return nil
	}
	flow := &nativeEnumParameterFlow{entry: g.Nodes[0], byPC: map[int]*core.OpCode{}, edges: map[*core.OpCode][]core.SemanticEdge{}, code: code}
	for _, op := range g.Nodes {
		if op == nil || op.Instr == nil || flow.byPC[int(op.CurrentOffset)] != nil {
			return nil
		}
		flow.byPC[int(op.CurrentOffset)] = op
	}
	for _, edge := range g.Edges {
		if edge.From == nil || edge.To == nil || flow.byPC[int(edge.From.CurrentOffset)] != edge.From || flow.byPC[int(edge.To.CurrentOffset)] != edge.To {
			return nil
		}
		flow.edges[edge.From] = append(flow.edges[edge.From], edge)
	}
	return flow
}

func (f *nativeEnumParameterFlow) parameterAt(read *nativeEnumSelectorProducer, work *workbudget.Budget) bool {
	if f == nil || read == nil || read.owner != "" || read.slot < 0 || read.slot > 65535 || read.result == "" || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(f.byPC))*96) != nil {
		return false
	}
	at := f.byPC[read.pc]
	if at == nil || at.Instr.OpCode != read.opcode || core.GetRetrieveIdx(at) != read.slot || !constructorMotionLoad(at, read.result) {
		return false
	}
	width := 1
	if read.result == "J" || read.result == "D" {
		width = 2
	}
	type state struct {
		op      *core.OpCode
		changed bool
	}
	pending := []state{{op: f.entry}}
	seen := map[state]bool{{op: f.entry}: true}
	reached := false
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current.op == nil || current.op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		if current.op == at {
			if current.changed {
				return false
			}
			reached = true
		}
		after := current.changed
		access := core.LocalAccessOf(current.op.Instr.OpCode)
		if access.Write {
			written := core.GetStoreIdx(current.op)
			if written < 0 || access.Width < 1 || access.Width > 2 {
				return false
			}
			after = after || written < read.slot+width && read.slot < written+access.Width
		}
		for _, edge := range f.edges[current.op] {
			if !nativeProofWork(work, 1) {
				return false
			}
			changed := after
			if edge.Kind == core.EdgeException {
				changed = current.changed
			}
			next := state{edge.To, changed}
			if !seen[next] {
				// Retain at most the two product states per node. Marking
				// after enqueue would permit many duplicate edge entries.
				seen[next] = true
				pending = append(pending, next)
			}
		}
	}
	return reached
}

func (f *nativeEnumParameterFlow) selector(node *nativeEnumSelectorProducer, work *workbudget.Budget, depth int) bool {
	if f == nil || node == nil || depth >= 32 || !nativeProofWork(work, 1) {
		return false
	}
	if node.owner == "" {
		if node.local != nil {
			pc, known := f.reachingStore(node, work)
			if !known || pc != node.local.storePC {
				return false
			}
		} else if !f.parameterAt(node, work) {
			return false
		}
	}
	for _, operand := range node.operands {
		if !f.selector(operand, work, depth+1) {
			return false
		}
	}
	return true
}
