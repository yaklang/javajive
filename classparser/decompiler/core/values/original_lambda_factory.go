package values

// This witness describes only the eager evaluation of an already decoded
// LambdaMetafactory site. It neither inspects the deferred body nor certifies
// renderer equivalence. Source-body/binding receipts remain a separate gate.
// Public Flag/PC/capture annotations alone cannot create the witness.
type originalLambdaFactory struct {
	pc                 int
	captures           []JavaValue
	methodRef, noOuter bool
	instantiated       string
}

// RequireLambdaParameterNames carries a completed body-source constraint.
// It grants no factory, operand, ownership, or effect permission. Source paths
// without this constraint retain their existing capture-snapshot protocol.
func (v *CustomValue) RequireLambdaParameterNames() {
	if v != nil {
		v.lambdaParameterNamesRequired = true
	}
}

func (v *CustomValue) LambdaParameterNamesRequired() bool {
	return v != nil && v.lambdaParameterNamesRequired
}

// MarkOriginalLambdaFactory is called by the validated lambda adapter after
// source construction. Retain exact eager operand identities and order; an
// unknown callback or a later changed capture cannot borrow this evidence.
func (v *CustomValue) MarkOriginalLambdaFactory(pc int) {
	if v == nil || v.originalLambdaFactory != nil || v.sourcePrimitive != nil || v.sourceCaught || v.Flag != "lambda" || !v.HasOriginPC || v.OriginPC != pc || pc < 0 || pc > 65535 || !v.CapturesKnown || len(v.Captures) > 64 || len(v.InstantiatedMtdDesc) > 65535 {
		return
	}
	for _, capture := range v.Captures {
		if isNilJavaValue(capture) {
			return
		}
	}
	v.originalLambdaFactory = &originalLambdaFactory{pc: pc, captures: append([]JavaValue(nil), v.Captures...), methodRef: v.IsMethodRef, noOuter: v.NoOuterCapture, instantiated: v.InstantiatedMtdDesc}
}

// OriginalLambdaFactoryEvaluation returns a defensive, bounded copy of eager
// operands. No String/Write/Type callback runs. Factory allocation/linkage can
// throw and must stay at this PC; a deferred body is never an eager operand.
func (v *CustomValue) OriginalLambdaFactoryEvaluation() (pc int, captures []JavaValue, known bool) {
	if v == nil || v.originalLambdaFactory == nil || v.sourcePrimitive != nil || v.sourceCaught || v.Flag != "lambda" || !v.HasOriginPC || !v.CapturesKnown {
		return 0, nil, false
	}
	w := v.originalLambdaFactory
	if v.OriginPC != w.pc || len(v.Captures) != len(w.captures) || v.IsMethodRef != w.methodRef || v.NoOuterCapture != w.noOuter || v.InstantiatedMtdDesc != w.instantiated {
		return 0, nil, false
	}
	for i, capture := range v.Captures {
		if isNilJavaValue(capture) || !sameOriginalValueIdentity(capture, w.captures[i]) {
			return 0, nil, false
		}
	}
	return w.pc, append([]JavaValue(nil), w.captures...), true
}

// A completed compiler-reference source packet constrains its receiver's
// source binding, independently of arrow/SAM parameter names. This requests
// only the existing original unchanged-copy proof, never factory permission.
func (v *CustomValue) RequireOriginalReferenceCapture() {
	if v != nil && v.IsMethodRef {
		v.originalReferenceCaptureRequired = true
	}
}
func (v *CustomValue) OriginalReferenceCaptureRequired() bool {
	return v != nil && v.IsMethodRef && v.originalReferenceCaptureRequired
}
