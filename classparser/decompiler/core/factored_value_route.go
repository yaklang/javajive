package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Factoring several branches into one predicate creates a synthetic value
// selector. It has no single branch opcode or taken/fallthrough polarity.
// Retain the proved routing entry and the two original terminal producers,
// separately from ConditionFromOp, which belongs to physical conditions.
type factoredValueRoute struct {
	entry     *OpCode
	condition values.JavaValue
	arms      [2]values.JavaValue
	terminals [2]*OpCode
}

// Called only after factorSharedValueTernary accepts the complete routing DAG.
func (d *Decompiler) recordFactoredValueRoute(selector *values.TernaryExpression, entry, yes, no *OpCode) {
	if d == nil || selector == nil || entry == nil || yes == nil || no == nil || yes == no || len(d.factoredValueRoutes) >= 512 ||
		d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || d.Work.CheckAlloc(int64(len(d.factoredValueRoutes)+1)*128) != nil) {
		return
	}
	if d.factoredValueRoutes == nil {
		d.factoredValueRoutes = map[*values.TernaryExpression]factoredValueRoute{}
	}
	d.factoredValueRoutes[selector] = factoredValueRoute{entry: entry, condition: selector.Condition, arms: [2]values.JavaValue{selector.TrueValue, selector.FalseValue}, terminals: [2]*OpCode{yes, no}}
}

func (d *Decompiler) factoredArraySourceArm(selector *values.TernaryExpression, call *values.FunctionCallExpression, allocation *OpCode) (values.JavaValue, *OpCode) {
	w, known := d.factoredValueRoutes[selector]
	if !known || selector == nil || selector.Condition != w.condition || selector.TrueValue != w.arms[0] || selector.FalseValue != w.arms[1] ||
		w.entry == nil || w.entry.Instr == nil || !isConditionalBranchOpcode(w.entry.Instr.OpCode) || !w.entry.TernaryChainArm ||
		!d.branchArrayOriginalPrefixPrecedes(w.entry, allocation) {
		return nil, nil
	}
	yes, yKnown := d.branchArrayCallOccurrences(w.arms[0], call)
	no, nKnown := d.branchArrayCallOccurrences(w.arms[1], call)
	if !yKnown || !nKnown || yes+no != 1 || call == nil || !call.HasOriginPC {
		return nil, nil
	}
	index := 0
	if no == 1 {
		index = 1
	}
	invoke := d.opcodeAtOffset(call.OriginPC)
	if invoke == nil || !sameBranchArrayInvocation(d.invokeFuncCall[invoke], call) ||
		!d.originalPrivateValueSuffix(allocation, invoke, w.terminals[index]) {
		return nil, nil
	}
	return w.arms[index], w.entry
}

// Every path after allocation must reach this invocation and then its original
// terminal without another decision or a new entry. Thus the folded predicate
// still chooses exactly the original allocation/call suffix. An arm value alone
// cannot prove that its producer belongs after all of the predicate's effects.
func (d *Decompiler) originalPrivateValueSuffix(first, via, last *OpCode) bool {
	if d == nil || d.semanticCFG == nil || first == nil || via == nil || last == nil || len(d.semanticCFG.Edges) > 65536 ||
		d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.semanticCFG.Edges))) != nil || d.Work.CheckAlloc(int64(len(d.semanticCFG.Edges)+1)*64) != nil) {
		return false
	}
	next, incoming := map[*OpCode][]*OpCode{}, map[*OpCode]int{}
	for _, edge := range d.semanticCFG.Edges {
		incoming[edge.To]++
		if edge.Kind != EdgeException {
			next[edge.From] = append(next[edge.From], edge.To)
		}
	}
	seenVia := false
	remaining := 65536
	for current, steps := first, 0; current != nil && steps < 512; steps++ {
		remaining -= len(d.ExceptionTable) + 1
		if remaining < 0 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.ExceptionTable))+1) != nil || !sameHandlerCoverage(d.handlersAt(first), d.handlersAt(current)) {
			return false
		}
		seenVia = seenVia || current == via
		if current == last {
			return seenVia
		}
		if len(next[current]) != 1 {
			return false
		}
		target := next[current][0]
		if target == nil || target.CurrentOffset <= current.CurrentOffset || incoming[target] != 1 {
			return false
		}
		current = target
	}
	return false
}
