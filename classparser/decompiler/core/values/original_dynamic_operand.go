package values

// A dynamic operand snapshot is a distinct declaration at the original
// invokedynamic site. It preserves one already evaluated operand in source
// evaluation order; equal names, types or values cannot confer this witness.
type originalDynamicOperand struct {
	pc, index int
	uid       string
	seed      JavaValue
	declared  bool
}

func (r *JavaRef) MarkOriginalDynamicOperand(pc, index int, seed JavaValue) {
	if r == nil || r.VarUid == "" || r.originalDynamicOperand != nil || pc < 0 || pc > 65535 || index < 0 || index >= 64 || isNilJavaValue(seed) || !sameOriginalValueIdentity(seed, r.Val) {
		return
	}
	r.originalDynamicOperand = &originalDynamicOperand{pc: pc, index: index, uid: r.VarUid, seed: seed}
}

func (r *JavaRef) MarkOriginalDynamicOperandDeclaration(pc int, seed JavaValue) {
	if r != nil && r.originalDynamicOperand != nil && r.VarUid == r.originalDynamicOperand.uid && pc == r.originalDynamicOperand.pc && sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) && sameOriginalValueIdentity(r.Val, seed) {
		r.originalDynamicOperand.declared = true
	}
}

func (r *JavaRef) OriginalDynamicOperandWitness(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || r.VarUid != r.originalDynamicOperand.uid || !r.originalDynamicOperand.declared || r.IsThis || r.IsParam || r.CustomValue != nil || r.StackVar != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}
