package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A source family that cannot regenerate an original compiler metadata role
// must not provisionally claim a lexical layout and block other families.
// This is a local capability refusal, not permission to discard metadata or
// substitute a body. The ordinary flat path still reports its unsupported body.
// A true result is only eligibility for further planning: rendering still
// validates every actual body/metadata requirement. Unknown roles grant nothing.
// Only an exact, exclusively used original lambda factory establishes the role;
// synthetic flags, method names and matching descriptors alone do not.
func nativeMemberCompilerMetadataEligible(root *ClassObject, family *nativeMemberFamily, profile SourceCompilerProfile, target int, work *workbudget.Budget) bool {
	if root == nil || family == nil || !nativeProofWork(work, 1) {
		return false
	}
	if profile == NativeJavac8 && target == 8 {
		return true
	}
	objects, known := nativeMemberDependencyObjects(root, family, work)
	if !known {
		return false
	}
	for _, object := range objects {
		if object == nil || !nativeProofWork(work, 1) {
			return false
		}
		for _, method := range object.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false
			}
			if method.AccessFlags != 0x100a && method.AccessFlags != 0x1002 {
				continue
			}
			metadata, known := readLambdaParameterMetadata(object, method, work)
			if known && metadata != nil && nativeLambdaBodyOriginalUseClosed(object, method, work, metadata) {
				// The current source renderer admits this exact physical parameter
				// table only under native javac8/source8. The profile decision is
				// independent of peer source names, body order and cache state.
				return false
			}
			if work != nil && work.Check() != nil {
				return false
			}
		}
	}
	return true
}
