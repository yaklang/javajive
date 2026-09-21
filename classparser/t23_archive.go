package javaclassparser

import (
	"context"

	"github.com/yaklang/javajive/internal/filesys"
	fi "github.com/yaklang/javajive/internal/filesys/filesys_interface"
	"github.com/yaklang/javajive/internal/workbudget"
)

// ArchiveOptionsFrom maps request-local DecompileOptions onto the archive reader.
func ArchiveOptionsFrom(opts DecompileOptions) filesys.ArchiveOptions {
	lim := filesys.ArchiveLimits{
		MaxArchiveEntries:    opts.Limits.MaxArchiveEntries,
		MaxArchiveEntryBytes: opts.Limits.MaxArchiveEntryBytes,
		MaxArchiveTotalBytes: opts.Limits.MaxArchiveTotalBytes,
		MaxArchiveDepth:      opts.Limits.MaxArchiveDepth,
	}
	work := opts.Work
	if work == nil {
		work = workbudget.New(opts.Context, opts.Limits)
	}
	return filesys.ArchiveOptions{
		Limits:        lim,
		Work:          work,
		Budget:        filesys.WrapWorkBudget(work, lim),
		Context:       opts.Context,
		TargetRelease: opts.TargetRelease,
		SafeArchive:   true,
	}
}

// archiveFSState is the request-scoped archive policy copied onto ExpandedZipFS/JarFS.
type archiveFSState struct {
	budget        *filesys.ArchiveBudget
	limits        filesys.ArchiveLimits
	ctx           context.Context
	depth         int
	targetRelease int
	safe          bool
	active        bool
}

func archiveStateFromZipFS(z *filesys.ZipFS) *archiveFSState {
	if z == nil || !z.ArchiveActive() {
		return nil
	}
	return &archiveFSState{
		budget:        z.ArchiveBudget(),
		limits:        z.ArchiveLimits(),
		ctx:           z.ArchiveContext(),
		depth:         z.ArchiveDepth(),
		targetRelease: z.TargetRelease(),
		safe:          z.SafeArchive() || z.ArchiveActive(),
		active:        true,
	}
}

func (s *archiveFSState) clone() *archiveFSState {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

func (s *archiveFSState) withDepth(depth int) *archiveFSState {
	cp := s.clone()
	if cp == nil {
		return &archiveFSState{depth: depth, active: true, safe: true}
	}
	cp.depth = depth
	cp.active = true
	cp.safe = true
	return cp
}

func (s *archiveFSState) zipOptions(depth int) []filesys.ZipFSOption {
	if s == nil || !s.active {
		return nil
	}
	opts := []filesys.ZipFSOption{
		filesys.WithArchiveLimits(s.limits),
		filesys.WithArchiveBudget(s.budget),
		filesys.WithArchiveDepth(depth),
		filesys.WithTargetRelease(s.targetRelease),
		filesys.WithSafeArchive(true),
	}
	if s.ctx != nil {
		opts = append(opts, filesys.WithArchiveContext(s.ctx))
	}
	return opts
}

func mergeArchiveState(base *archiveFSState, opts filesys.ArchiveOptions) *archiveFSState {
	s := base.clone()
	if s == nil {
		s = &archiveFSState{}
	}
	if opts.Budget != nil {
		s.budget = opts.Budget
	}
	if opts.Limits != (filesys.ArchiveLimits{}) {
		s.limits = opts.Limits
	}
	if opts.Context != nil {
		s.ctx = opts.Context
	}
	if opts.Depth != 0 {
		s.depth = opts.Depth
	}
	s.targetRelease = opts.TargetRelease
	s.safe = true
	s.active = true
	if s.budget == nil && opts.Limits != (filesys.ArchiveLimits{}) {
		s.budget = filesys.NewArchiveBudget(opts.Context, s.limits)
	}
	return s
}

func (s *archiveFSState) cacheKey(path string) string {
	rel := 0
	if s != nil {
		rel = s.targetRelease
	}
	return filesys.ArchiveCacheKey(path, rel)
}

func (s *archiveFSState) chargeNestedDepth() error {
	if s == nil || !s.active {
		return nil
	}
	next := int64(s.depth + 1)
	if s.budget != nil {
		return s.budget.Charge(filesys.CounterArchiveDepth, next)
	}
	if s.limits.MaxArchiveDepth > 0 && int(next) > s.limits.MaxArchiveDepth {
		return &filesys.ArchiveError{
			Kind:    filesys.KindResource,
			Counter: filesys.CounterArchiveDepth,
			Used:    next,
			Limit:   int64(s.limits.MaxArchiveDepth),
		}
	}
	return nil
}

func (s *archiveFSState) nestedZipFSOptions() []filesys.ZipFSOption {
	if s == nil || !s.active {
		return nil
	}
	return s.zipOptions(s.depth + 1)
}

func (s *archiveFSState) nestedState() *archiveFSState {
	if s == nil {
		return nil
	}
	return s.withDepth(s.depth + 1)
}

// NewExpandedZipFSWithArchiveOptions wraps NewExpandedZipFSWithOptions and attaches
// a shared archive budget/limits/targetRelease used for nested jar/zip opens.
func NewExpandedZipFSWithArchiveOptions(underlying fi.FileSystem, zipFS *filesys.ZipFS, recursiveParse bool, opts filesys.ArchiveOptions) *ExpandedZipFS {
	e := NewExpandedZipFSWithOptions(underlying, zipFS, recursiveParse)
	e.archive = mergeArchiveState(e.archive, opts)
	return e
}

func (e *ExpandedZipFS) archiveCacheKey(path string) string {
	if e == nil {
		return filesys.ArchiveCacheKey(path, 0)
	}
	return e.archive.cacheKey(path)
}

func (e *ExpandedZipFS) chargeNestedArchive() error {
	if e == nil {
		return nil
	}
	return e.archive.chargeNestedDepth()
}

func (e *ExpandedZipFS) nestedZipFSOptions() []filesys.ZipFSOption {
	if e == nil {
		return nil
	}
	if e.archive != nil && e.archive.active {
		return e.archive.nestedZipFSOptions()
	}
	if e.zipFS != nil && e.zipFS.ArchiveActive() {
		return e.zipFS.NestedArchiveOptions(e.zipFS.ArchiveDepth() + 1)
	}
	return nil
}

func (e *ExpandedZipFS) applyMultiRelease(name string) string {
	if e == nil || e.archive == nil || e.archive.targetRelease == 0 {
		return name
	}
	var entries []string
	if e.zipFS != nil {
		entries = e.zipFS.EntryNames()
	}
	if chosen, ok := filesys.SelectMultiReleasePath(entries, name, e.archive.targetRelease); ok {
		return chosen
	}
	return name
}

func (z *JarFS) archiveCacheKey(path string) string {
	if z == nil {
		return filesys.ArchiveCacheKey(path, 0)
	}
	return z.archive.cacheKey(path)
}

func (z *JarFS) chargeNestedArchive() error {
	if z == nil {
		return nil
	}
	return z.archive.chargeNestedDepth()
}

func (z *JarFS) nestedZipFSOptions() []filesys.ZipFSOption {
	if z == nil {
		return nil
	}
	if z.archive != nil && z.archive.active {
		return z.archive.nestedZipFSOptions()
	}
	if z.ZipFS != nil && z.ZipFS.ArchiveActive() {
		return z.ZipFS.NestedArchiveOptions(z.ZipFS.ArchiveDepth() + 1)
	}
	return nil
}
