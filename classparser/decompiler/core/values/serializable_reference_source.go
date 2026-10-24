package values

type serializableReferenceSource struct {
	token any
	pc    int
}

// The token joins a final retained value to an original physical factory in
// its capturing class. It certifies neither the target binding nor a renderer;
// those remain separate source-packet obligations in the producer.
func NewSerializableReferenceSource(value *CustomValue, token any) *CustomValue {
	if value == nil || token == nil || !value.IsMethodRef || !value.NoOuterCapture || value.serializableReferenceSource != nil {
		return nil
	}
	pc, captures, known := value.OriginalLambdaFactoryEvaluation()
	if !known || len(captures) != 0 {
		return nil
	}
	copy := *value
	// Serializable factories use their own completed-source sink. Preserve
	// the canonical body, then let that separately certified sink observe its
	// actual writer; it cannot also impersonate an ordinary-field receipt.
	if copy.sourceMethodReference != nil {
		copy.WriteFunc = copy.sourceMethodReference.write
		copy.sourceMethodReference = nil
	}
	copy.serializableReferenceSource = &serializableReferenceSource{token: token, pc: pc}
	return &copy
}

func (value *CustomValue) SerializableReferenceSource() (any, bool) {
	if value == nil || value.serializableReferenceSource == nil || !value.IsMethodRef || !value.NoOuterCapture {
		return nil, false
	}
	pc, captures, known := value.OriginalLambdaFactoryEvaluation()
	if !known || len(captures) != 0 || pc != value.serializableReferenceSource.pc {
		return nil, false
	}
	return value.serializableReferenceSource.token, true
}
