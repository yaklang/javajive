package javaclassparser

import (
	"bytes"
	"crypto/sha256"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A namespace-local original input owns its parsed declaration. The modern
// nest and the later lexical family must refer to that same declaration;
// equal class names or independently parsed pointers grant no permission.
// This private node is prepared before any source projection is published.
type nativeModernNestInput struct {
	object *ClassObject
	digest [32]byte
	size   int
}

func sealNativeModernNestInput(raw []byte, work *workbudget.Budget) ([]byte, nativeModernNestInput, bool) {
	if len(raw) < 10 || !nativeProofWork(work, int64(len(raw))*2+1) || work != nil && work.CheckAlloc(int64(len(raw))+128) != nil {
		return nil, nativeModernNestInput{}, false
	}
	// The resolver may own its buffer. Retain a private input before parsing,
	// so another read cannot change the recorded original's Code slices.
	owned := bytes.Clone(raw)
	return owned, nativeModernNestInput{digest: sha256.Sum256(owned), size: len(owned)}, true
}

func (input nativeModernNestInput) bind(name string, raw []byte, work *workbudget.Budget) (*ClassObject, bool) {
	if input.object == nil || input.object.GetClassName() != name || len(raw) != input.size || !nativeProofWork(work, int64(len(raw))+1) || sha256.Sum256(raw) != input.digest {
		return nil, false
	}
	// Reuse only the private original declaration whose entire input was
	// revalidated in the same resolver namespace. Keep all original nest,
	// exclusive-use, source-layout and publication checks at their consumers.
	return input.object, true
}
