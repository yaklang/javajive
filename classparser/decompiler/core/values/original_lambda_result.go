package values

// An eager lambda-result temporary is distinct from a JVM local and from a
// DUP materialization. Giving it a separate receipt avoids changing the older
// type-sharing and stack-motion permissions of those roles.
type originalLambdaResult struct {
	ref   *JavaRef
	uid   string
	pc    int
	value *CustomValue
}

func (ref *JavaRef) MarkOriginalLambdaResult(pc int, value *CustomValue) {
	if ref == nil || ref.originalLambdaResult != nil || value == nil || ref.Id == nil || ref.VarUid == "" || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.originalLocalDeclaration || ref.originalDynamicOperand != nil || ref.Val != value {
		return
	}
	producerPC, _, known := value.OriginalLambdaFactoryEvaluation()
	if known && producerPC == pc {
		ref.originalLambdaResult = &originalLambdaResult{ref: ref, uid: ref.VarUid, pc: pc, value: value}
	}
}

func (ref *JavaRef) OriginalLambdaResult(seed JavaValue) (pc int, known bool) {
	if ref == nil || ref.originalLambdaResult == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.originalLocalDeclaration || ref.originalDynamicOperand != nil {
		return 0, false
	}
	w := ref.originalLambdaResult
	if ref != w.ref && !ref.SharesSourceDeclaration(w.ref) || ref.VarUid != w.uid || ref.Id != w.ref.Id || ref.Val != w.value {
		return 0, false
	}
	for steps := 0; steps < 32; steps++ {
		if slot, ok := seed.(*SlotValue); ok && slot != nil {
			seed = slot.GetValue()
			continue
		}
		producerPC, _, original := w.value.OriginalLambdaFactoryEvaluation()
		return w.pc, seed == w.value && original && producerPC == w.pc
	}
	return 0, false
}
