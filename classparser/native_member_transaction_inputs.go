package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// Count retained parsed models, including foreign caller models. Distinct
// parses of the same original name each occupy memory; deduplicating by binary
// name would hide their amplification in a large source component. Shared
// pointers within one plan are counted once. This is resource accounting alone.
type nativeMemberTransactionInputs struct {
	models map[*ClassObject]bool
	bytes  int64
}

func (a *nativeMemberTransactionInputs) add(z *JarFS, plan *nativeMemberPrepared, work *workbudget.Budget) bool {
	if a == nil || z == nil || plan == nil || plan.family == nil || plan.root == nil || plan.family.lexicalObjects[plan.root.GetClassName()] != plan.root || !nativeProofWork(work, 1) {
		return false
	}
	for _, objects := range []map[string]*ClassObject{plan.objects, plan.family.lexicalObjects, plan.family.modernNestObjects} {
		for name, object := range objects {
			if object == nil || object.GetClassName() != name || !nativeProofWork(work, int64(len(name))+1) {
				return false
			}
			if a.models[object] {
				continue
			}
			if len(a.models) >= 4096 {
				return false
			}
			raw, known := z.enumSiblingResolver()(name)
			if !known || len(raw) > 2<<20 || int64(len(raw)) > (128<<20)-a.bytes || !nativeProofWork(work, int64(len(raw))+1) {
				return false
			}
			next := a.bytes + int64(len(raw))
			if work != nil && work.CheckAlloc(next+int64(len(a.models)+1)*64) != nil {
				return false
			}
			if a.models == nil {
				a.models = map[*ClassObject]bool{}
			}
			a.models[object] = true
			a.bytes = next
		}
	}
	return true
}
