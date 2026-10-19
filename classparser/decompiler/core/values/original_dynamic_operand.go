package values

// A dynamic operand snapshot is a distinct declaration at the original
// invokedynamic site. It preserves one already evaluated operand in source
// evaluation order; equal names, types or values cannot confer this witness.
type originalDynamicOperand struct {
	pc, index int
	uid       string
	seed      JavaValue
	declared  bool
	inlined   bool
}

func (r *JavaRef) MarkOriginalDynamicOperand(pc, index int, seed JavaValue) {
	if r == nil || r.VarUid == "" || r.originalDynamicOperand != nil || pc < 0 || pc > 65535 || index < 0 || index >= 64 || isNilJavaValue(seed) || !sameOriginalValueIdentity(seed, r.Val) {
		return
	}
	r.originalDynamicOperand = &originalDynamicOperand{pc: pc, index: index, uid: r.VarUid, seed: seed}
}

func (r *JavaRef) MarkOriginalDynamicOperandDeclaration(pc int, seed JavaValue) {
	if r != nil && r.originalDynamicOperand != nil && !r.originalDynamicOperand.inlined && r.VarUid == r.originalDynamicOperand.uid && pc == r.originalDynamicOperand.pc && sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) && sameOriginalValueIdentity(r.Val, seed) {
		r.originalDynamicOperand.declared = true
	}
}

// The adjacent-consumer proof can retain a capture as an explicit StackVar
// instead of emitting its snapshot declaration. Record that source projection
// separately: it is neither a declaration nor permission to follow arbitrary
// ref.Val histories or same-spelled forwarding closures.
func (r *JavaRef) MarkOriginalDynamicOperandInline(pc int, seed JavaValue) {
	if r == nil || r.originalDynamicOperand == nil || r.originalDynamicOperand.declared || r.originalDynamicOperand.inlined || r.Id == nil || r.VarUid != r.originalDynamicOperand.uid || r.IsThis || r.IsParam || r.CustomValue != nil || pc != r.originalDynamicOperand.pc || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) || !sameOriginalValueIdentity(r.StackVar, seed) {
		return
	}
	r.originalDynamicOperand.inlined = true
}

func (r *JavaRef) OriginalInlinedDynamicOperandWitness(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || !r.originalDynamicOperand.inlined || r.originalDynamicOperand.declared || r.Id == nil || r.VarUid != r.originalDynamicOperand.uid || r.IsThis || r.IsParam || r.CustomValue != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) || !sameOriginalValueIdentity(r.StackVar, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}

func (r *JavaRef) OriginalDynamicOperandWitness(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || r.VarUid != r.originalDynamicOperand.uid || !r.originalDynamicOperand.declared || r.IsThis || r.IsParam || r.CustomValue != nil || r.StackVar != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}

// Pending evaluation identity exists before a source declaration or inline
// projection is selected. Completed projections cannot borrow this phase. This query grants neither source binding nor motion permission;
// consumers still need an independent original producer and final projection.
func (r *JavaRef) OriginalPendingDynamicOperand(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || r.originalDynamicOperand.declared || r.originalDynamicOperand.inlined || r.StackVar != nil || r.Id == nil || r.VarUid != r.originalDynamicOperand.uid || r.IsThis || r.IsParam || r.CustomValue != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}

// Statement construction copies the logical snapshot ref. Require the same
// private original operand witness and current source binding, rather than
// pointer equality with the earlier stack-simulation copy or name equality.
func (r *JavaRef) OriginalDynamicOperandDeclarationOf(snapshot *JavaRef, seed JavaValue) bool {
	if r == nil || snapshot == nil || r.Id == nil || r.Id != snapshot.Id || r.originalDynamicOperand == nil || r.originalDynamicOperand != snapshot.originalDynamicOperand {
		return false
	}
	_, _, declared := r.OriginalDynamicOperandWitness(seed)
	_, _, captured := snapshot.OriginalDynamicOperandWitness(snapshot.Val)
	return declared && captured
}
