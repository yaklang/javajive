package core

import (
	"fmt"
	"sort"
)

// EdgeKind records JVM flow, independently of the mutable structuring graph.
type EdgeKind uint8

const (
	EdgeFallthrough EdgeKind = iota
	EdgeTaken
	EdgeCase
	EdgeDefault
	EdgeException
)

type SemanticEdge struct {
	From, To     *OpCode
	Kind         EdgeKind
	CaseValue    int32
	HandlerOrder int
}

// SemanticCFG is a method-local snapshot taken before expression/region rewrites.
// Exception edges leave the throwing instruction's INPUT local state. They are
// never confused with the synthetic try-entry anchors used by the old structurer.
type SemanticCFG struct {
	Nodes      []*OpCode
	Edges      []SemanticEdge
	incoming   map[*OpCode][]int
	outgoing   map[*OpCode][]int
	slots      map[int]*slotDefinitions
	Updates    int
	order      map[*OpCode]int
	Err        error
	MaxUpdates int
}

// MayThrowOpcode conservatively includes resolution/linkage, allocation, array,
// monitor and invocation failures, not only explicit athrow instructions.
func MayThrowOpcode(op int) bool {
	switch op {
	case OP_IDIV, OP_LDIV, OP_IREM, OP_LREM, OP_ATHROW,
		OP_IALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD, OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD,
		OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE,
		OP_GETFIELD, OP_PUTFIELD, OP_GETSTATIC, OP_PUTSTATIC,
		OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKESTATIC, OP_INVOKEINTERFACE, OP_INVOKEDYNAMIC,
		OP_NEW, OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY, OP_ARRAYLENGTH,
		OP_CHECKCAST, OP_INSTANCEOF, OP_MONITORENTER, OP_MONITOREXIT, OP_LDC, OP_LDC_W, OP_LDC2_W:
		return true
	}
	return false
}

func (d *Decompiler) buildSemanticCFG() (*SemanticCFG, error) {
	if err := d.validateControlFlow(); err != nil {
		return nil, err
	}
	g := &SemanticCFG{order: map[*OpCode]int{}, MaxUpdates: 1000000, incoming: map[*OpCode][]int{}, outgoing: map[*OpCode][]int{}, slots: map[int]*slotDefinitions{}}
	if d.MaxAnalysisUpdates > 0 {
		g.MaxUpdates = d.MaxAnalysisUpdates
	}
	for _, op := range d.opCodes {
		if op.Instr.OpCode != OP_START && op.Instr.OpCode != OP_END {
			g.order[op] = len(g.Nodes)
			g.Nodes = append(g.Nodes, op)
		}
	}
	target := func(pc int) *OpCode { return d.opCodes[d.offsetToOpcodeIndex[uint16(pc)]] }
	add := func(from, to *OpCode, kind EdgeKind, key int32, order int) {
		i := len(g.Edges)
		g.Edges = append(g.Edges, SemanticEdge{from, to, kind, key, order})
		g.incoming[to] = append(g.incoming[to], i)
		g.outgoing[from] = append(g.outgoing[from], i)
	}
	for i, op := range g.Nodes {
		fall := true
		switch op.Instr.OpCode {
		case OP_RETURN, OP_IRETURN, OP_LRETURN, OP_FRETURN, OP_DRETURN, OP_ARETURN, OP_ATHROW:
			fall = false
		case OP_GOTO, OP_GOTO_W:
			add(op, target(op.BranchTarget), EdgeTaken, 0, 0)
			fall = false
		case OP_JSR, OP_JSR_W, OP_RET:
			return nil, fmt.Errorf("semantic CFG requires inlined jsr/ret at PC %d", op.CurrentOffset)
		case OP_LOOKUPSWITCH, OP_TABLESWITCH:
			fall = false
			op.SwitchJmpCase.ForEach(func(v int, pc int32) bool { add(op, target(int(pc)), EdgeCase, int32(v), 0); return true })
			add(op, target(int(op.SwitchDefaultOffset)), EdgeDefault, 0, 0)
		case OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE, OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL:
			add(op, target(op.BranchTarget), EdgeTaken, 0, 0)
		}
		if fall && i+1 < len(g.Nodes) {
			add(op, g.Nodes[i+1], EdgeFallthrough, 0, 0)
		}
		if MayThrowOpcode(op.Instr.OpCode) {
			for order, h := range d.ExceptionTable {
				if op.CurrentOffset >= h.StartPc && op.CurrentOffset < h.EndPc {
					add(op, target(int(h.HandlerPc)), EdgeException, 0, order)
					if h.CatchType == 0 {
						break
					} // ordered catch-all shadows subsequent handlers
				}
			}
		}
	}
	if err := g.ValidateReducible(); err != nil {
		return nil, err
	}
	return g, nil
}

type definitionSet map[*OpCode]struct{} // nil is the stable method-entry definition
// Each slot is solved once on the immutable graph, then every load/store query is O(defs).
// This avoids repeated reverse walks and makes loop joins independent of DFS order.
type slotDefinitions struct {
	in        map[*OpCode]definitionSet
	reachable map[*OpCode]bool
}

func (g *SemanticCFG) solveSlot(slot int) *slotDefinitions {
	if cached := g.slots[slot]; cached != nil {
		return cached
	}
	facts := &slotDefinitions{in: map[*OpCode]definitionSet{}, reachable: map[*OpCode]bool{}}
	if len(g.Nodes) == 0 {
		g.slots[slot] = facts
		return facts
	}
	entry := g.Nodes[0]
	facts.in[entry] = definitionSet{nil: {}}
	facts.reachable[entry] = true
	queue := []*OpCode{entry}
	queued := map[*OpCode]bool{entry: true}
	for head := 0; head < len(queue); head++ {
		if g.Err != nil {
			break
		}
		if g.MaxUpdates > 0 && g.Updates >= g.MaxUpdates {
			g.Err = fmt.Errorf("analysis_budget_exceeded: reaching definitions after %d updates", g.Updates)
			break
		}
		op := queue[head]
		queued[op] = false
		g.Updates++
		before := facts.in[op]
		after := before
		access := LocalAccessOf(op.Instr.OpCode)
		if access.Write {
			at := GetStoreIdx(op)
			if at == slot {
				after = definitionSet{op: {}}
			} else if access.Width == 2 && at+1 == slot {
				after = definitionSet{nil: {}}
			} else if at == slot+1 {
				// Writing a category-2 tail invalidates its head, but must not
				// kill unrelated category-1 definitions at that head slot.
				after = definitionSet{}
				for def := range before {
					if def != nil && LocalAccessOf(def.Instr.OpCode).Width == 2 {
						after[nil] = struct{}{}
					} else {
						after[def] = struct{}{}
					}
				}
			}
		}
		for _, ei := range g.outgoing[op] {
			edge := g.Edges[ei]
			state := after
			if edge.Kind == EdgeException {
				state = before
			}
			changed := !facts.reachable[edge.To]
			facts.reachable[edge.To] = true
			into := facts.in[edge.To]
			if into == nil {
				into = definitionSet{}
				facts.in[edge.To] = into
			}
			for def := range state {
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
	g.slots[slot] = facts
	return facts
}

// ReachingDefinitions returns definition identities in bytecode order, including
// iinc's read/write identity. The separate entry flag is not a fabricated store.
func (g *SemanticCFG) ReachingDefinitions(at *OpCode, slot int) ([]*OpCode, bool) {
	facts := g.solveSlot(slot)
	set := facts.in[at]
	out := make([]*OpCode, 0, len(set))
	_, entry := set[nil]
	for op := range set {
		if op != nil {
			out = append(out, op)
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entry
}

// reachingStores is the adapter used by existing JavaRef/web consumers. iinc
// updates the same source variable; follow its input definitions rather than
// requiring the old simulator to invent a second JavaRef for the increment.
func (d *Decompiler) reachingStores(at *OpCode, slot int) ([]*OpCode, bool) {
	if d.semanticCFG == nil {
		return reachingStoresOf(at, slot)
	}
	g := d.semanticCFG
	pending := []*OpCode{at}
	seen := map[*OpCode]bool{}
	defs := map[*OpCode]bool{}
	entry := false
	for len(pending) > 0 {
		cur := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stores, e := g.ReachingDefinitions(cur, slot)
		entry = entry || e
		for _, store := range stores {
			if store.Instr.OpCode == OP_IINC {
				pending = append(pending, store)
			} else {
				defs[store] = true
			}
		}
	}
	out := make([]*OpCode, 0, len(defs))
	for op := range defs {
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entry
}
