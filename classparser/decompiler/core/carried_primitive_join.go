package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A conditional can preserve the current numeric stack word on one edge and
// replace it on the other. Prove the original closed, forward update region;
// neither equal stack heights nor a source-level method name proves a phi.
// The bypass is the conditional's own edge, so attaching an ordinary edge copy
// after that conditional would execute the copy on both successors.
func (d *Decompiler) carriedPrimitiveRoutingRoot(merge *OpCode, candidates []*OpCode) *OpCode {
	if merge == nil || len(merge.Source) != 2 || merge.IsCatch || merge.IsTryCatchParent || len(d.handlersAt(merge)) != 0 || len(candidates) > 128 {
		return nil
	}
	for _, root := range candidates {
		if root == nil || root.Instr == nil || !isConditionalBranchOpcode(root.Instr.OpCode) || len(root.Target) != 2 || root.StackEntry == nil || root.StackEntry.depth != 1 || len(d.handlersAt(root)) != 0 {
			continue
		}
		if root.Target[0] != merge && root.Target[1] != merge {
			continue
		}
		if root.Target[0] == root.Target[1] {
			continue
		}
		seed, ok := retainedJoinValue(root.StackEntry.value)
		if !ok || !carriedNumericType(seed) {
			continue
		}
		// Pure local arithmetic can be evaluated immediately before its condition.
		// Heap reads, calls and opaque values require an earlier lifetime snapshot;
		// preserveStackAcrossWrite already supplies those for materialized prefixes.
		if effects, _ := values.InspectValue(seed); effects != 0 {
			continue
		}
		next := root.Target[0]
		if next == merge {
			next = root.Target[1]
		}
		previous := root
		valid := true
		steps := 0
		for next != merge {
			if d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || steps >= 512 || next == nil || next.Instr == nil || next.IsCatch || next.IsTryCatchParent || len(d.handlersAt(next)) != 0 || next.CurrentOffset <= previous.CurrentOffset || next.CurrentOffset >= merge.CurrentOffset || len(next.Source) != 1 || next.Source[0] != previous || len(next.Target) != 1 || stackLifetimeWrite(next.Instr.OpCode) {
				valid = false
				break
			}
			previous, next = next, next.Target[0]
			steps++
		}
		if valid && steps > 0 && ((merge.Source[0] == root && merge.Source[1] == previous) || (merge.Source[1] == root && merge.Source[0] == previous)) {
			return root
		}
	}
	return nil
}

func carriedNumericType(v values.JavaValue) bool {
	if v == nil || v.Type() == nil {
		return false
	}
	p, ok := v.Type().RawType().(*types.JavaPrimer)
	if !ok {
		return false
	}
	switch p.Name {
	case types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
		return true
	}
	return false
}

// Initialize a fresh local before the conditional, and replace it only on the
// unique updating edge. This is ordinary phi destruction with a bypass edge:
// every effect remains in its original arm and the common continuation runs
// exactly once. Proof completes before publishing any source statements.
func (d *Decompiler) lowerCarriedPrimitiveJoin(merge, root *OpCode, slot *values.SlotValue) bool {
	if root == nil || slot == nil || d.carriedPrimitiveRoutingRoot(merge, []*OpCode{root}) != root {
		return false
	}
	seed, ok := retainedJoinValue(root.StackEntry.value)
	if !ok {
		return false
	}
	pred := merge.Source[0]
	if pred == root {
		pred = merge.Source[1]
	}
	if pred.StackEntry == nil || pred.StackEntry.depth != 1 || !d.sameStackLifetimePrefix(root.StackEntry.parent, pred.StackEntry.parent) {
		return false
	}
	updated, ok := retainedJoinValue(pred.StackEntry.value)
	if !ok || !carriedNumericType(updated) || updated.Type().RawType().(*types.JavaPrimer).Name != seed.Type().RawType().(*types.JavaPrimer).Name {
		return false
	}
	sim := d.opcodeToSimulateStack[merge]
	if sim == nil || d.effectfulStackPhiEdges[pred] != nil {
		return false
	}
	if d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return false
	}
	typ := seed.Type().Copy()
	ref := sim.NewVar(seed)
	ref.ResetVarType(typ)
	ref.WebDeclType = typ.Copy()
	if d.carriedPrimitivePhiSeeds == nil {
		d.carriedPrimitivePhiSeeds = map[*OpCode]*statements.AssignStatement{}
	}
	d.carriedPrimitivePhiSeeds[root] = statements.NewAssignStatement(ref, seed, true)
	if d.effectfulStackPhiEdges == nil {
		d.effectfulStackPhiEdges = map[*OpCode]*statements.AssignStatement{}
	}
	assign := statements.NewAssignStatement(ref, pred.StackEntry.value, false)
	assign.OriginPC, assign.HasOriginPC = int(pred.CurrentOffset), true
	d.effectfulStackPhiEdges[pred] = assign
	d.disFoldRef = append(d.disFoldRef, ref)
	// The new initializer/edge write owns real uses absent from simulation.
	// Do not let an old single-use callback retire those original definitions.
	for _, value := range []values.JavaValue{seed, pred.StackEntry.value} {
		_, reads := values.InspectValue(value)
		for read := range reads {
			d.disFoldRef = append(d.disFoldRef, read)
		}
	}
	slot.ResetValue(ref)
	return true
}

// The initializer and conditional share one original opcode. Keep the exact
// certified writer before that branch when instruction packets become nodes;
// name/shape equality cannot enroll some unrelated assignment or condition.
func (d *Decompiler) carriedPrimitiveSourceChain(chain []*Node, primary *Node, op *OpCode) bool {
	if len(chain) != 2 || chain[1] != primary || op == nil || !isConditionalBranchOpcode(op.Instr.OpCode) || d.carriedPrimitivePhiSeeds[op] == nil {
		return false
	}
	if chain[0].Statement != d.carriedPrimitivePhiSeeds[op] || chain[0].Id != primary.Id {
		return false
	}
	_, ok := primary.Statement.(*statements.ConditionStatement)
	return ok
}
