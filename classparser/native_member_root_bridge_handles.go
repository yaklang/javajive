package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A root access bridge can be replaced by lexical constructor access only when
// no method handle observes any root constructor descriptor. Non-constructor
// targets use the ordinary exact-declaration proof; synthetic private lambda
// targets require its original exclusive-metafactory certificate. Neither an
// unused CP entry nor a foreign lookup gets an exemption from these proofs.
func nativeRootBridgeMethodHandlesClosed(root *ClassObject, index *nativeMemberIndex, source *nativeMemberClass, work *workbudget.Budget) bool {
	if root == nil || index == nil || !index.valid || source == nil || source.object != root || !nativeProofWork(work, 1) {
		return false
	}
	targets := index.handleTargets[root.GetClassName()]
	if len(targets) > 4096 || !nativeProofWork(work, int64(len(targets))) {
		return false
	}
	for _, target := range targets {
		if target.kind == 8 || target.name == "<init>" || target.name == "<clinit>" {
			return false
		}
	}
	return nativeMemberOrdinaryHandlesClosed(root, index, work, source)
}
