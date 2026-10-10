package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A value return immediately following a proved release consumes a value
// already computed while the monitor was held. Its expression cannot be emitted
// after release: a field read could observe a later writer, and a call/cast could
// escape the original monitor's exception domain. Preserve that stack value in
// a dedicated source temporary before releasing, without moving Java tokens
// across the monitor. The immutable CFG proves the closed release corridor to the return;
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
	// Trace backwards from the return across a closed release corridor. Nested
	// synchronized blocks have ALOAD; MONITOREXIT for each enclosing owner:
	// capture before the FIRST release, not merely the last one.
	predecessor := func(n *OpCode) *OpCode {
		if len(g.incoming[n]) != 1 {
			return nil
		}
		edge := g.Edges[g.incoming[n][0]]
		if edge.Kind != EdgeFallthrough {
			return nil
		}
		from := edge.From
		normal := 0
		for _, index := range g.outgoing[from] {
			if g.Edges[index].Kind != EdgeException {
				normal++
			}
		}
		if normal != 1 {
			return nil
		}
		return from
	}
	ownedRelease := func(n *OpCode) bool {
		_, known := owners[n]
		return known && n != nil && !n.IsCustom && n.Instr != nil && n.Instr.OpCode == OP_MONITOREXIT
	}
	for _, ret := range g.Nodes {
		if d.Work != nil {
			if err := d.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
				return nil, nil, err
			}
		}
		if ret == nil || ret.IsCustom || ret.Instr == nil || len(ret.stackConsumed) != 1 {
			continue
		}
		exit := predecessor(ret)
		if !ownedRelease(exit) {
			continue
		}
		// Every intermediate edge is unique and every release already has a
		// proved original acquisition. No call, arithmetic, cast, stack shuffle,
		// branch or exception entry can be crossed by this plan.
		for {
			if d.Work != nil {
				if err := d.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
					return nil, nil, err
				}
			}
			load := predecessor(exit)
			if load == nil || load.IsCustom || load.Instr == nil || !isReferenceLoadOpcode(load.Instr.OpCode) {
				exit = nil
				break
			}
			earlier := predecessor(load)
			if !ownedRelease(earlier) {
				// An unproved release or a joined release corridor cannot
				// silently become a snapshot after the inner lock was lost.
				for _, index := range g.incoming[load] {
					from := g.Edges[index].From
					if from != nil && from.Instr != nil && from.Instr.OpCode == OP_MONITOREXIT {
						exit = nil
					}
				}
				break
			}
			exit = earlier
		}
		if exit == nil {
			continue
		}
		value := ret.stackConsumed[0]
		// JVM computational categories, not source spelling, determine the
		// returned word(s). In particular Z/B/S/C/I all consume an int word;
		// J/D each consume a category-2 value, not two independent operands.
		if value == nil || d.FunctionType == nil || !originalMonitorReturnCategory(ret.Instr.OpCode, value.Type(), d.FunctionType.ReturnType) {
			continue
		}
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

// Match both the operand and declared result to the return instruction. This
// proves width as well as category; unknown, void and method types cannot grant
// a snapshot. Reference assignability remains the existing return binder's job.
func originalMonitorReturnCategory(op int, operand, result types.JavaType) bool {
	category := func(t types.JavaType) int {
		if t == nil {
			return -1
		}
		switch raw := t.RawType().(type) {
		case *types.JavaPrimer:
			switch raw.Name {
			case types.JavaBoolean, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger:
				return OP_IRETURN
			case types.JavaLong:
				return OP_LRETURN
			case types.JavaFloat:
				return OP_FRETURN
			case types.JavaDouble:
				return OP_DRETURN
			}
		case *types.JavaClass, *types.JavaArrayType, *types.JavaParameterizedType:
			return OP_ARETURN
		}
		return -1
	}
	return op != -1 && category(operand) == op && category(result) == op
}
