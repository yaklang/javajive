package class_context

import (
	"maps"
	"unsafe"

	"github.com/yaklang/javajive/internal/workbudget"
)

// SnapshotForSourceProof retains the actual method coordinates after a shared
// class context advances to a sibling. Resolver functions, value identities and
// the monotone request budget remain shared; mutable name/type/import containers
// are copied. Preflight both copy work and storage before CloneForRetry allocates.
func (f *ClassContext) SnapshotForSourceProof(work *workbudget.Budget) (*ClassContext, bool) {
	if f == nil {
		return nil, false
	}
	units := int64(1)
	for _, n := range []int{
		len(f.Arguments), len(f.TypeParams), len(f.ClassTypeParams), len(f.LexicalTypeParamSignatures),
		len(f.InjectedTypeParamBounds), len(f.FieldTypeVars), len(f.FieldSignatures),
		len(f.MethodSignatures), len(f.MethodSignaturesByDesc), len(f.MethodDescriptors), len(f.PoolMethodDescriptors),
		len(f.sameArityOverloadMemo), len(f.ConstructorSignatures), len(f.ConstructorSignaturesByDesc),
		len(f.RawEraseTypeVars), len(f.StandaloneEraseTypeVars), len(f.ForceParamEraseTypeVars),
		len(f.SamePkgFQNames), len(f.LexicalTypeNames), len(f.staticImportScopeMemo), len(f.LocalNames), len(f.CatchEntryNames),
	} {
		units += int64(n)
	}
	if f.KeySet != nil {
		units += int64(f.KeySet.Len())
	}
	if f.StaticMethodImports != nil {
		units += int64(len(f.StaticMethodImports.owners))
	}
	if f.BuildInLibsMap != nil {
		if work != nil && work.Charge(workbudget.CounterGraphScans, int64(f.BuildInLibsMap.Len())) != nil {
			return nil, false
		}
		f.BuildInLibsMap.ForEach(func(_ string, names []string) bool {
			units += 1 + int64(len(names))
			return true
		})
	}
	bytes, err := workbudget.CheckedProduct(units, 128)
	if err != nil || bytes > int64(^uint64(0)>>1)-int64(unsafe.Sizeof(*f)) {
		return nil, false
	}
	if work != nil && (work.CheckAlloc(bytes+int64(unsafe.Sizeof(*f))) != nil || work.Charge(workbudget.CounterNodeCopies, units) != nil) {
		return nil, false
	}
	out := f.CloneForRetry()
	out.LocalNames = maps.Clone(f.LocalNames)
	out.CatchEntryNames = maps.Clone(f.CatchEntryNames)
	out.SourceAllocationRender, out.SourceConstructedInvocation = nil, nil
	return out, true
}
