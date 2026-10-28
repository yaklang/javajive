package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// INEG has a numeric computational result even when a branch-local 0/1 seed
// was inferred as boolean. Keep the exact original operation and operand;
// this only proves the source domain of its complete reaching-definition web.
func originalIntNegationResult(op *OpCode) (*values.JavaExpression, bool) {
	if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INEG || op.IsCustom || op.IsWide || len(op.Data) != 0 || len(op.stackConsumed) != 1 || len(op.stackProduced) != 1 {
		return nil, false
	}
	x, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaExpression)
	if !ok || x == nil || x.Op != values.SUB || len(x.Values) != 1 || !isExactPrimer(x.Type(), types.JavaInteger) || x.Values[0] == nil || op.stackConsumed[0] == nil || values.OriginalStackLifetimeUse(x.Values[0]) != values.OriginalStackLifetimeUse(op.stackConsumed[0]) {
		return nil, false
	}
	return x, true
}

// Stack duplication preserves the evaluated word's domain. A materialized
// reference is evidence only with its sealed original DUP packet, never merely
// JavaRef.Val or its inferred type. Build explicit dependencies and propagate
// proven numeric roots once per edge; cycles and unknown roots prove nothing.
func (d *Decompiler) extendNumericStackProducers(producers map[values.JavaValue]bool) map[values.JavaValue]bool {
	if d == nil || d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.opCodes) > 8192 || len(d.semanticCFG.Nodes) > 8192 {
		return producers
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opCodes)+len(d.semanticCFG.Nodes))) != nil || d.Work.CheckAlloc(int64(len(d.opCodes)+len(d.semanticCFG.Nodes))*192) != nil) {
		return nil
	}
	original, decoded := map[*OpCode]bool{}, map[*OpCode]bool{}
	for _, op := range d.semanticCFG.Nodes {
		original[op] = true
	}
	for _, op := range d.opCodes {
		decoded[op] = true
	}
	known := func(op *OpCode) bool {
		return op != nil && op.Instr != nil && !op.IsCustom && original[op] && decoded[op]
	}
	dependents := map[values.JavaValue][]*values.JavaRef{}
	for _, op := range d.opCodes {
		if !known(op) {
			continue
		}
		if x := d.originalIntFieldPostResult(op, known); x != nil {
			producers[x] = true
		}
		if op.Instr.OpCode != OP_DUP || op.IsWide || len(op.Data) != 0 || len(op.stackConsumed) != 1 || len(op.stackProduced) != 2 {
			continue
		}
		infos, converted := d.opcodeIdToRef[op], d.dupConvertedRefValue[op]
		if len(infos) != 1 || infos[0][1] != true || len(converted) != 1 || converted[0] == nil || converted[0] != op.stackConsumed[0] {
			continue
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil {
			continue
		}
		pc, code, sealed := ref.OriginalStackMaterializationWitness(converted[0])
		cell := op.stackProduced[0]
		if !sealed || pc != int(op.CurrentOffset) || code != OP_DUP || cell == nil || op.stackProduced[1] != cell || safeSeedOperand(cell) != ref || d.stackValueProducers[cell] != op {
			continue
		}
		source := safeSeedOperand(converted[0])
		if source != nil {
			dependents[source] = append(dependents[source], ref)
		}
	}
	queue := make([]values.JavaValue, 0, len(producers))
	for value, proved := range producers {
		if proved {
			queue = append(queue, value)
		}
	}
	for i := 0; i < len(queue); i++ {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(1+len(dependents[queue[i]]))) != nil {
			return nil
		}
		for _, ref := range dependents[queue[i]] {
			if !producers[ref] {
				producers[ref] = true
				queue = append(queue, ref)
			}
		}
	}
	return producers
}

// The simulator can retain a field's old value as one post-update expression.
// Its numeric domain needs the exact original read, arithmetic, and write of
// that same field/receiver. SelfOpFolded and a mutable expression type alone
// cannot certify this. No expression is substituted or moved by this proof.
func (d *Decompiler) originalIntFieldPostResult(op *OpCode, known func(*OpCode) bool) *values.JavaExpression {
	if !known(op) || !op.SelfOpFolded || op.IsWide || len(op.Data) != 2 || len(op.stackProduced) != 1 || d.constantPoolGetter == nil {
		return nil
	}
	instance := op.Instr.OpCode == OP_PUTFIELD
	if !instance && op.Instr.OpCode != OP_PUTSTATIC || instance && len(op.stackConsumed) != 3 || !instance && len(op.stackConsumed) != 2 {
		return nil
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.Description != "I" {
		return nil
	}
	x, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaExpression)
	if !ok || x == nil || len(x.Values) != 2 || !isExactPrimer(x.Type(), types.JavaInteger) || x.Op != values.INC && x.Op != values.DEC {
		return nil
	}
	stored := values.OriginalStackLifetimeUse(op.stackConsumed[0])
	arithmetic := d.stackValueProducers[stored]
	if !known(arithmetic) {
		return nil
	}
	rhs, proved := originalIntArithmeticResult(arithmetic)
	if !proved || safeSeedOperand(stored) != rhs || (x.Op == values.INC && rhs.Op != values.ADD) || (x.Op == values.DEC && rhs.Op != values.SUB) || !isLiteralIntOne(rhs.Values[1]) || x.Values[1] != rhs.Values[1] {
		return nil
	}
	old := safeSeedOperand(op.stackConsumed[len(op.stackConsumed)-1])
	if old == nil || old != safeSeedOperand(rhs.Values[0]) {
		return nil
	}
	ref, ok := old.(*values.JavaRef)
	if !ok || ref == nil {
		return nil
	}
	// Recover the seed through the recorded materialization, never a search
	// through arbitrary local references. The witness also rejects a stale Val.
	pc, code, sealed := ref.OriginalStackMaterializationWitness(x.Values[0])
	if !sealed || instance && code != OP_DUP_X1 || !instance && code != OP_DUP {
		return nil
	}
	duplicate := d.stackValueProducers[values.OriginalStackLifetimeUse(op.stackConsumed[len(op.stackConsumed)-1])]
	if !known(duplicate) || int(duplicate.CurrentOffset) != pc || duplicate.Instr.OpCode != code || duplicate.IsWide || len(duplicate.Data) != 0 {
		return nil
	}
	infos, converted := d.opcodeIdToRef[duplicate], d.dupConvertedRefValue[duplicate]
	if len(infos) != 1 || infos[0][0] != ref || infos[0][1] != true || len(converted) != 1 || safeSeedOperand(converted[0]) != x.Values[0] {
		return nil
	}
	oldCell := values.OriginalStackLifetimeUse(op.stackConsumed[len(op.stackConsumed)-1])
	if instance {
		if len(duplicate.stackConsumed) != 3 || len(duplicate.stackProduced) != 4 || duplicate.stackConsumed[0] != converted[0] || duplicate.stackConsumed[1] != oldCell || duplicate.stackProduced[0] != oldCell || duplicate.stackProduced[1] != oldCell || duplicate.stackProduced[3] != oldCell || duplicate.stackProduced[2] != duplicate.stackConsumed[2] || values.OriginalStackLifetimeUse(duplicate.stackConsumed[2]) != values.OriginalStackLifetimeUse(op.stackConsumed[1]) {
			return nil
		}
	} else if len(duplicate.stackConsumed) != 1 || len(duplicate.stackProduced) != 2 || duplicate.stackConsumed[0] != converted[0] || duplicate.stackProduced[0] != oldCell || duplicate.stackProduced[1] != oldCell {
		return nil
	}
	read := d.stackValueProducers[values.OriginalStackLifetimeUse(converted[0])]
	if !known(read) || read.IsWide || len(read.Data) != 2 || len(read.stackProduced) != 1 || safeSeedOperand(read.stackProduced[0]) != x.Values[0] {
		return nil
	}
	readMember, ok := d.constantPoolGetter(int(Convert2bytesToInt(read.Data))).(*values.JavaClassMember)
	if !ok || readMember == nil || readMember.Name != member.Name || readMember.Member != member.Member || readMember.Description != member.Description {
		return nil
	}
	if instance {
		field, ok := x.Values[0].(*values.RefMember)
		if !ok || field == nil || read.Instr.OpCode != OP_GETFIELD || len(read.stackConsumed) != 1 || !field.OriginalInstanceFieldRead(int(read.CurrentOffset), member.Name, member.Member, member.Description) || values.OriginalStackLifetimeUse(field.Object) != values.OriginalStackLifetimeUse(op.stackConsumed[1]) || values.OriginalStackLifetimeUse(read.stackConsumed[0]) != values.OriginalStackLifetimeUse(op.stackConsumed[1]) {
			return nil
		}
	} else {
		field, ok := x.Values[0].(*values.JavaClassMember)
		if !ok || field == nil || read.Instr.OpCode != OP_GETSTATIC || len(read.stackConsumed) != 0 || !field.OriginalStaticFieldRead(int(read.CurrentOffset), member.Name, member.Member, member.Description) {
			return nil
		}
	}
	return x
}
