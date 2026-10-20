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
