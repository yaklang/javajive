package javaclassparser

import (
	"strconv"
	"strings"
	"sync"
)

// A physical version path identifies a source namespace, not another alias
// for the base class's cached source. The name must be canonical and the
// class-file binary identity is checked by the caller before entering a view.
func physicalClassNamespace(name string) (int, string, bool) {
	rest, found := strings.CutPrefix(name, "META-INF/versions/")
	if !found {
		return 0, "", false
	}
	version, logical, found := strings.Cut(rest, "/")
	release, err := strconv.Atoi(version)
	if !found || err != nil || release < 9 || strconv.Itoa(release) != version || !strings.HasSuffix(logical, ".class") || !nativeSourceBinaryName(strings.TrimSuffix(logical, ".class")) {
		return 0, "", false
	}
	return release, logical, true
}

func (z *JarFS) sourceReleaseView(release int) *JarFS {
	if z == nil || release < 9 {
		return nil
	}
	z.releaseViewsMu.Lock()
	defer z.releaseViewsMu.Unlock()
	if view := z.releaseViews[release]; view != nil {
		return view
	}
	// A bounded number of immutable namespace views. All retain the archive's
	// original shared work/output budget and their own source ownership cache.
	if len(z.releaseViews) >= 16 {
		return nil
	}
	zip := z.ZipFS.ReleaseView(release)
	if zip == nil {
		return nil
	}
	view := NewJarFSWithOptions(zip, z.recursiveParse)
	view.archive = z.archive.clone()
	if view.archive != nil {
		view.archive.targetRelease = release
	}
	view.declarationResolver = z.declarationResolver
	view.sourceOwnership = z.sourceOwnership
	if z.releaseViews == nil {
		z.releaseViews = map[int]*JarFS{}
	}
	z.releaseViews[release] = view
	return view
}

// All release views share one retained-source allowance, as well as the
// archive's work budget. Creating a namespace must not multiply memory limits.
type sourceOwnershipCache struct {
	mu              sync.Mutex
	bytes           int64
	dependencyBytes int64 // shared across all release views and source policies
}

func (z *JarFS) reserveOwnershipDependencyMetadata(size int64) bool {
	if size < 0 || z == nil || z.sourceOwnership == nil {
		return false
	}
	ledger := z.sourceOwnership
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if size > (16<<20)-ledger.dependencyBytes {
		return false
	}
	total := ledger.dependencyBytes + size
	if z.archive != nil && z.archive.budget != nil && z.archive.budget.Work().CheckAlloc(total) != nil {
		return false
	}
	ledger.dependencyBytes = total
	return true
}

func (z *JarFS) reserveOwnershipSource(size int64) bool {
	if size < 0 || z == nil || z.sourceOwnership == nil {
		return false
	}
	ledger := z.sourceOwnership
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if size > (16<<20)-ledger.bytes {
		return false
	}
	total := ledger.bytes + size
	if z.archive != nil && z.archive.budget != nil && z.archive.budget.Work().CheckAlloc(total) != nil {
		return false
	}
	ledger.bytes = total
	return true
}
