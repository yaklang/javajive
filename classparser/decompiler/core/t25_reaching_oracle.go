package core

import (
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// independentDefSet mirrors production definitionSet without sharing its type.
type independentDefSet map[*OpCode]struct{}

func cloneIndep(src independentDefSet) independentDefSet {
	out := independentDefSet{}
	for k := range src {
		out[k] = struct{}{}
	}
	return out
}

// independentCategoryWidth is a JVM-spec local-variable width table that does
// not consult LocalAccessOf.
func independentCategoryWidth(opcode int) int {
	switch opcode {
	case OP_LLOAD, OP_LSTORE,
		OP_LLOAD_0, OP_LLOAD_1, OP_LLOAD_2, OP_LLOAD_3,
		OP_LSTORE_0, OP_LSTORE_1, OP_LSTORE_2, OP_LSTORE_3,
		OP_DLOAD, OP_DSTORE,
		OP_DLOAD_0, OP_DLOAD_1, OP_DLOAD_2, OP_DLOAD_3,
		OP_DSTORE_0, OP_DSTORE_1, OP_DSTORE_2, OP_DSTORE_3:
		return 2
	default:
		return 1
	}
}

func independentIsLocalWrite(opcode int) bool {
	switch opcode {
	case OP_ISTORE, OP_LSTORE, OP_FSTORE, OP_DSTORE, OP_ASTORE, OP_IINC,
		OP_ISTORE_0, OP_ISTORE_1, OP_ISTORE_2, OP_ISTORE_3,
		OP_LSTORE_0, OP_LSTORE_1, OP_LSTORE_2, OP_LSTORE_3,
		OP_FSTORE_0, OP_FSTORE_1, OP_FSTORE_2, OP_FSTORE_3,
		OP_DSTORE_0, OP_DSTORE_1, OP_DSTORE_2, OP_DSTORE_3,
		OP_ASTORE_0, OP_ASTORE_1, OP_ASTORE_2, OP_ASTORE_3:
		return true
	default:
		return false
	}
}

func independentIndexedSlot(op *OpCode, opcode int, base, base0 int) int {
	if opcode >= base0 && opcode <= base0+3 {
		return opcode - base0
	}
	if opcode != base && opcode != OP_IINC {
		return -1
	}
	if op == nil {
		return -1
	}
	if op.IsWide {
		if len(op.Data) < 2 {
			return -1
		}
		return int(binary.BigEndian.Uint16(op.Data))
	}
	if len(op.Data) == 0 {
		return -1
	}
	return int(op.Data[0])
}

// independentLocalWrite decodes a local store/iinc without GetStoreIdx.
func independentLocalWrite(op *OpCode) (slot, width int, ok bool) {
	if op == nil || op.Instr == nil {
		return -1, 1, false
	}
	code := op.Instr.OpCode
	if !independentIsLocalWrite(code) {
		return -1, 1, false
	}
	width = independentCategoryWidth(code)
	switch {
	case code == OP_IINC || code == OP_ISTORE || (code >= OP_ISTORE_0 && code <= OP_ISTORE_3):
		return independentIndexedSlot(op, code, OP_ISTORE, OP_ISTORE_0), width, true
	case code == OP_LSTORE || (code >= OP_LSTORE_0 && code <= OP_LSTORE_3):
		return independentIndexedSlot(op, code, OP_LSTORE, OP_LSTORE_0), width, true
	case code == OP_FSTORE || (code >= OP_FSTORE_0 && code <= OP_FSTORE_3):
		return independentIndexedSlot(op, code, OP_FSTORE, OP_FSTORE_0), width, true
	case code == OP_DSTORE || (code >= OP_DSTORE_0 && code <= OP_DSTORE_3):
		return independentIndexedSlot(op, code, OP_DSTORE, OP_DSTORE_0), width, true
	case code == OP_ASTORE || (code >= OP_ASTORE_0 && code <= OP_ASTORE_3):
		return independentIndexedSlot(op, code, OP_ASTORE, OP_ASTORE_0), width, true
	default:
		return -1, 1, false
	}
}

func independentTypeWidth(t types.JavaType) int {
	if t == nil {
		return 1
	}
	s := t.String(&class_context.ClassContext{})
	if s == types.JavaLong || s == types.JavaDouble {
		return 2
	}
	return 1
}

// independentLiveInWidth walks FunctionType/IsStatic without production entryWidth.
func independentLiveInWidth(g *SemanticCFG, slot int) int {
	if g == nil {
		return 0
	}
	cur := 0
	if !g.IsStatic {
		if slot == 0 {
			return 1
		}
		cur = 1
	}
	if g.FunctionType == nil {
		return 0
	}
	for _, p := range g.FunctionType.ParamTypes {
		w := independentTypeWidth(p)
		if w < 1 {
			w = 1
		}
		if cur == slot {
			return w
		}
		cur += w
	}
	return 0
}

func independentLiveInIsCat2Tail(g *SemanticCFG, slot int) bool {
	if slot <= 0 {
		return false
	}
	return independentLiveInWidth(g, slot-1) == 2
}

func independentDefIsCat2(g *SemanticCFG, def *OpCode, solvedSlot int) bool {
	if def == nil {
		return independentLiveInWidth(g, solvedSlot) == 2
	}
	if def.Instr == nil {
		return false
	}
	return independentCategoryWidth(def.Instr.OpCode) == 2
}

func independentEntrySet(g *SemanticCFG, slot int) independentDefSet {
	if independentLiveInIsCat2Tail(g, slot) {
		return independentDefSet{}
	}
	return independentDefSet{nil: {}}
}

// IndependentReachingOracle is a slow, always-clone worklist used only as the
// T25 CFG reference. It must not call SemanticCFG.solveSlot, SparseReaching,
// LocalAccessOf, or GetStoreIdx.
func IndependentReachingOracle(g *SemanticCFG, at *OpCode, slot int) ([]*OpCode, bool, uint64) {
	if g == nil || len(g.Nodes) == 0 || at == nil {
		return nil, false, 0
	}
	in := map[*OpCode]independentDefSet{}
	reachable := map[*OpCode]bool{}
	entry := g.Nodes[0]
	in[entry] = independentEntrySet(g, slot)
	reachable[entry] = true
	queue := []*OpCode{entry}
	queued := map[*OpCode]bool{entry: true}
	var copies uint64
	for head := 0; head < len(queue); head++ {
		op := queue[head]
		queued[op] = false
		before := in[op]
		after := before
		writeSlot, writeWidth, isWrite := independentLocalWrite(op)
		if isWrite {
			if writeSlot == slot {
				after = independentDefSet{op: {}}
			} else if writeWidth == 2 && writeSlot+1 == slot {
				after = independentDefSet{nil: {}}
			} else if writeSlot == slot+1 {
				after = independentDefSet{}
				for def := range before {
					copies++
					if independentDefIsCat2(g, def, slot) {
						after[nil] = struct{}{}
					} else {
						after[def] = struct{}{}
					}
				}
			} else {
				after = cloneIndep(before)
				copies += uint64(len(before))
			}
		} else {
			after = cloneIndep(before)
			copies += uint64(len(before))
		}
		for _, ei := range g.outgoing[op] {
			edge := g.Edges[ei]
			state := after
			if edge.Kind == EdgeException {
				state = before
			}
			changed := !reachable[edge.To]
			reachable[edge.To] = true
			into := in[edge.To]
			if into == nil {
				into = independentDefSet{}
				in[edge.To] = into
			}
			for def := range state {
				copies++
				if _, ok := into[def]; !ok {
					into[def] = struct{}{}
					changed = true
				}
			}
			if changed && !queued[edge.To] {
				queued[edge.To] = true
				queue = append(queue, edge.To)
			}
		}
	}
	set := in[at]
	out := make([]*OpCode, 0, len(set))
	entryFlag := false
	for op := range set {
		if op == nil {
			entryFlag = true
			continue
		}
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entryFlag, copies
}

// cat2Cell is occupancy of one local slot for the linear category-2 interpreter.
type cat2Cell struct {
	def       *OpCode
	fromEntry bool
	cat2      bool
	present   bool
}

// LinearCat2Occupancy walks nodes in bytecode order (no CFG worklist, no
// transferReaching, no LocalAccessOf) and applies JVM local occupancy:
// live-in long/double identity, tail writes kill the head, cat2 stores occupy
// two slots. Used as an independent reference for method-entry wide parameters.
func LinearCat2Occupancy(g *SemanticCFG, at *OpCode, slot int) ([]*OpCode, bool) {
	if g == nil || at == nil {
		return nil, false
	}
	cells := map[int]cat2Cell{}
	cur := 0
	if g != nil && !g.IsStatic {
		cells[0] = cat2Cell{fromEntry: true, present: true, cat2: false}
		cur = 1
	}
	if g != nil && g.FunctionType != nil {
		for _, p := range g.FunctionType.ParamTypes {
			w := independentTypeWidth(p)
			if w < 1 {
				w = 1
			}
			cells[cur] = cat2Cell{fromEntry: true, present: true, cat2: w == 2}
			cur += w
		}
	} else if _, ok := cells[0]; !ok {
		cells[0] = cat2Cell{fromEntry: true, present: true}
	}
	for _, n := range g.Nodes {
		if n == at {
			break
		}
		ws, ww, wr := independentLocalWrite(n)
		if !wr || ws < 0 {
			continue
		}
		cells[ws] = cat2Cell{def: n, present: true, cat2: ww == 2}
		if ww == 2 {
			delete(cells, ws+1)
		}
		if head, ok := cells[ws-1]; ok && head.present && head.cat2 {
			cells[ws-1] = cat2Cell{fromEntry: true, present: true, cat2: false}
		}
	}
	c := cells[slot]
	if !c.present {
		return nil, false
	}
	if c.def == nil {
		return nil, true
	}
	return []*OpCode{c.def}, false
}
