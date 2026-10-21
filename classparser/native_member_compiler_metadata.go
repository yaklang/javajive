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
func nativeMemberCompilerMetadataEligible(root *ClassObject, family *nativeMemberFamily, profile SourceCompilerProfile, target int, work *workbudget.Budget, authorities ...*sourceLambdaMetadataAuthority) bool {
	if root == nil || family == nil || len(authorities) > 1 || !nativeProofWork(work, 1) {
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
				// Plain source needs native javac8/source8. An explicit artifact
				// instead owes a final, sealed postcompile table obligation. This
				// decision is independent of peer names and source-cache state.
				if len(authorities) != 1 || authorities[0] == nil || profile.effective() != ModernJavac || target != 8 || object.MajorVersion != 52 || method.AccessFlags != 0x100a {
					return false
				}
				if _, bound := authorities[0].original(object, work); !bound || !sourceLambdaOrdinaryBootstrapClosed(object, work) {
					return false
				}
				// Eligibility permits planning only. Publication still requires
				// each actual final source/body/factory obligation to close.
			}
			if work != nil && work.Check() != nil {
				return false
			}
		}
	}
	return true
}
