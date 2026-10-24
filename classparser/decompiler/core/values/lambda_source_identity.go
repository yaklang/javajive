package values

// LambdaSourceIdentity names one original declaration whose source bodies have
// completed their separate reconstruction receipts. Capture-specific cache
// entries can share this deferred program; eager captures and SAM adaptation
// still need independent comparison. Equal text or names grant no identity.
type LambdaSourceIdentity struct{ self *LambdaSourceIdentity }

func NewLambdaSourceIdentity() *LambdaSourceIdentity {
	id := &LambdaSourceIdentity{}
	id.self = id
	return id
}

func (v *CustomValue) BindLambdaSourceIdentity(id *LambdaSourceIdentity) {
	if v != nil && v.lambdaSourceIdentity == nil && id != nil && id.self == id && v.Flag == "lambda" && !v.IsMethodRef {
		v.lambdaSourceIdentity = id
	}
}

// BindOriginalLambdaFactorySignature runs only after adapter validation. The
// ordinary metafactory's SAM name, factory descriptor and erased SAM are part
// of its identity, independently of its deferred implementation body.
func (v *CustomValue) BindOriginalLambdaFactorySignature(name, descriptor, erased string) {
	if v == nil || v.originalLambdaFactory == nil || v.originalLambdaFactory.signature != [3]string{} || name == "" || descriptor == "" || erased == "" || len(name)+len(descriptor)+len(erased) > 1<<18 {
		return
	}
	v.originalLambdaFactory.signature = [3]string{name, descriptor, erased}
}

// SameOriginalLambdaSource compares two deferred programs without invoking a
// renderer or type callback. A caller must still compare eager captures in
// order and prove the exception domains of both factory evaluations.
func (v *CustomValue) SameOriginalLambdaSource(other *CustomValue) bool {
	if v == nil || other == nil || v.lambdaSourceIdentity == nil || v.lambdaSourceIdentity != other.lambdaSourceIdentity || v.lambdaSourceIdentity.self != v.lambdaSourceIdentity || v.IsMethodRef || other.IsMethodRef {
		return false
	}
	_, _, a := v.OriginalLambdaFactoryEvaluation()
	_, _, b := other.OriginalLambdaFactoryEvaluation()
	return a && b && v.originalLambdaFactory.signature[0] != "" && v.originalLambdaFactory.signature == other.originalLambdaFactory.signature && v.InstantiatedMtdDesc == other.InstantiatedMtdDesc && v.NoOuterCapture == other.NoOuterCapture
}
