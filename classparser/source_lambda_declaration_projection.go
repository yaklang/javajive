package javaclassparser

import (
	"crypto/sha256"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Metadata clients share the same original/source-bound alpha conversion.
// The returned map grants only a declaration identity: each client still
// validates the complete fresh universe, flags and its own attribute contract.
// Names are compared as lossless class-file MUTF-8, not rendered Java spelling.
func sourceLambdaDeclarationProjection(want *ClassObject, original []byte, plan *sourceLambdaMetadataPlan, work *workbudget.Budget, flags func(sourceLambdaMethodIdentity) (uint16, bool)) (map[sourceLambdaMethodIdentity]sourceLambdaMethodIdentity, error) {
	if plan == nil {
		return nil, nil
	}
	fail := func(reason string) (map[sourceLambdaMethodIdentity]sourceLambdaMethodIdentity, error) {
		return nil, sourceLambdaMetadataFailure(work, reason)
	}
	if want == nil || flags == nil || plan.owner != want.GetClassName() || plan.originalHash != sha256.Sum256(original) || plan.sourceHash == ([32]byte{}) || len(plan.methods) == 0 || len(plan.methods) > 4096 {
		return fail("unpublished or stale lambda projection")
	}
	if err := work.CheckAlloc(int64(len(plan.methods)) * 256); err != nil {
		return nil, err
	}
	projection := map[sourceLambdaMethodIdentity]sourceLambdaMethodIdentity{}
	originals := map[sourceLambdaMethodIdentity]bool{}
	identity := func(key sourceLambdaMethodIdentity) sourceLambdaMethodIdentity {
		return sourceLambdaMethodIdentity{string(NewUtf8FromString(key.name).mutf8Bytes()), string(NewUtf8FromString(key.descriptor).mutf8Bytes())}
	}
	for _, method := range plan.methods {
		if !nativeProofWork(work, int64(len(method.original.name)+len(method.original.descriptor)+len(method.generated.name)+len(method.generated.descriptor))+1) {
			return fail("lambda projection budget")
		}
		old, next := identity(method.original), identity(method.generated)
		f, known := flags(old)
		if method.original.descriptor != method.generated.descriptor || projection[next].name != "" || originals[old] || !known || !sourceLambdaImplementationFlags(f) {
			return fail("ambiguous lambda projection")
		}
		projection[next] = old
		originals[old] = true
	}
	return projection, nil
}
