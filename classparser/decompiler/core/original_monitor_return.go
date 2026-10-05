package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A reference return immediately following a proved release consumes a value
// already computed while the monitor was held. Its expression cannot be emitted
// after release: a field read could observe a later writer, and a call/cast could
// escape the original monitor's exception domain. Preserve that stack value in
// a dedicated source temporary before releasing, without moving Java tokens
// across the monitor. The immutable CFG proves the exact exit/return edge;
// mutable ref.Val and statement spelling do not establish this relationship.
func (d *Decompiler) snapshotOriginalMonitorReturns(owners map[*OpCode]int) (map[*OpCode]*values.JavaRef, map[*OpCode]EvaluationSnapshot, error) {
	out := map[*OpCode]*values.JavaRef{}
	snapshots := map[*OpCode]EvaluationSnapshot{}
	g := d.semanticCFG
	if g == nil || g.Err != nil || len(owners) == 0 {
		return out, snapshots, nil
	}
	if d.Work != nil {
		if err := d.Work.CheckAlloc(int64(len(g.Nodes)) * 128); err != nil {
			return nil, nil, err
		}
	}
	for _, exit := range g.Nodes {
		if d.Work != nil {
			if err := d.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
				return nil, nil, err
			}
		}
		if _, known := owners[exit]; !known || exit == nil || exit.IsCustom || exit.Instr == nil || exit.Instr.OpCode != OP_MONITOREXIT {
			continue
		}
		var ret *OpCode
		valid := true
		for _, i := range g.outgoing[exit] {
			e := g.Edges[i]
			if e.Kind == EdgeException {
				continue
			}
			if e.Kind != EdgeFallthrough || ret != nil {
				valid = false
				break
			}
			ret = e.To
		}
		if !valid || ret == nil || ret.IsCustom || ret.Instr == nil || ret.Instr.OpCode != OP_ARETURN || len(ret.stackConsumed) != 1 {
			continue
		}
		// A join/handler must not inherit the snapshot of a different predecessor.
		if len(g.incoming[ret]) != 1 || g.Edges[g.incoming[ret][0]].From != exit || g.Edges[g.incoming[ret][0]].Kind != EdgeFallthrough {
			continue
		}
		value := ret.stackConsumed[0]
		access := values.InspectAccess(value)
		if access.Effects == 0 || access.Effects&values.EffectOpaque != 0 {
			continue
		}
		sim := d.opcodeToSimulateStack[exit]
		if sim == nil || value.Type() == nil {
			continue
		}
		if err := d.chargeNodeCopies(1); err != nil {
			return nil, nil, err
		}
		ref := sim.NewVar(value)
		ref.ResetVarType(ref.Type().Copy())
		d.disFoldRef = append(d.disFoldRef, ref)
		if d.evaluationSnapshots == nil {
			d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{}
		}
		snapshots[exit] = EvaluationSnapshot{Ref: ref, Value: value, OriginPC: int(exit.CurrentOffset)}
		d.evaluationSnapshots[exit] = append(d.evaluationSnapshots[exit], snapshots[exit])
		out[ret] = ref
	}
	return out, snapshots, nil
}

// The general opcode emission combiner joins only plain assignments. A monitor
// snapshot additionally has its own exact two-node plan: capture then release.
// Do not relax the combiner for arbitrary control-flow or opaque statements.
func originalMonitorSnapshotChain(chain []*Node, primary *Node, op *OpCode, owners map[*OpCode]int, snapshots map[*OpCode]EvaluationSnapshot) bool {
	if len(chain) != 2 || chain[1] != primary || chain[0] == nil || primary == nil || op == nil || op.IsCustom || op.Instr == nil || op.Instr.OpCode != OP_MONITOREXIT {
		return false
	}
	snapshot, known := snapshots[op]
	if !known {
		return false
	}
	owner, known := owners[op]
	if !known {
		return false
	}
	exit, ok := primary.Statement.(*statements.MiddleStatement)
	if !ok {
		return false
	}
	pc, paired, known := exit.OriginalMonitor()
	if !known || exit.Flag != "monitor_exit" || paired != owner || pc != snapshot.OriginPC || pc != int(op.CurrentOffset) {
		return false
	}
	assign, ok := chain[0].Statement.(*statements.AssignStatement)
	return ok && assign != nil && assign.LeftValue == snapshot.Ref && assign.JavaValue == snapshot.Value && assign.ArrayMember == nil && assign.IsFirst && assign.HasOriginPC && assign.OriginPC == pc
}
