package javaclassparser

// ReleasePreparationCaches drops archive/declaration/source-ownership graphs
// after all prepare workers have joined. Materialized Java belongs to the
// caller's virtual filesystem, not to these caches. Subsequent reads may reopen
// archives and reconstruct the same proofs. This is a phase-boundary operation;
// callers must not run it concurrently with filesystem reads.
func (e *ExpandedZipFS) ReleasePreparationCaches() {
	if e == nil {
		return
	}
	e.jarCache.Clear()
	e.zipCache.Clear()
	e.declarationsMu.Lock()
	e.declarations = nil
	e.declarationsMu.Unlock()
	releaseAllClassDumpScratch()
}
