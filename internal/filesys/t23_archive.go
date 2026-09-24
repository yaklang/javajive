package filesys

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/internal/log"
	"github.com/yaklang/javajive/internal/memfile"
	"github.com/yaklang/javajive/internal/utils"
	"github.com/yaklang/javajive/internal/workbudget"
	zip "github.com/yaklang/javajive/internal/zipx"
)

const (
	CounterArchiveEntries    = "archive_entries"
	CounterArchiveEntryBytes = "archive_entry_bytes"
	CounterArchiveTotalBytes = "archive_total_bytes"
	CounterArchiveDepth      = "archive_depth"

	KindResource = "resource_limit"
	KindCanceled = "canceled"
	KindInvalid  = "invalid_input"

	CodeArchivePathRejected = "archive_path_rejected"
)

// ArchiveLimits bounds catalog, decompressed bytes, and nest depth.
// Zero fields are unlimited. Negative fields are invalid_input.
type ArchiveLimits struct {
	MaxArchiveEntries    int64
	MaxArchiveEntryBytes int64
	MaxArchiveTotalBytes int64
	MaxArchiveDepth      int
}

func (l ArchiveLimits) Validate() error {
	if l.MaxArchiveEntries < 0 || l.MaxArchiveEntryBytes < 0 || l.MaxArchiveTotalBytes < 0 || l.MaxArchiveDepth < 0 {
		return &ArchiveError{Kind: KindInvalid, Err: errors.New("negative archive limit")}
	}
	return nil
}

func (l ArchiveLimits) limited() bool {
	return l.MaxArchiveEntries > 0 || l.MaxArchiveEntryBytes > 0 || l.MaxArchiveTotalBytes > 0 || l.MaxArchiveDepth > 0
}

func (l ArchiveLimits) ToWorkLimits() workbudget.Limits {
	return workbudget.Limits{
		MaxArchiveEntries:    l.MaxArchiveEntries,
		MaxArchiveEntryBytes: l.MaxArchiveEntryBytes,
		MaxArchiveTotalBytes: l.MaxArchiveTotalBytes,
		MaxArchiveDepth:      l.MaxArchiveDepth,
	}
}

func ArchiveLimitsFromWork(l workbudget.Limits) ArchiveLimits {
	return ArchiveLimits{
		MaxArchiveEntries:    l.MaxArchiveEntries,
		MaxArchiveEntryBytes: l.MaxArchiveEntryBytes,
		MaxArchiveTotalBytes: l.MaxArchiveTotalBytes,
		MaxArchiveDepth:      l.MaxArchiveDepth,
	}
}

// ArchiveOptions is the request-scoped archive policy passed into ZipFS constructors.
type ArchiveOptions struct {
	Limits        ArchiveLimits
	Budget        *ArchiveBudget
	Work          *workbudget.Budget // canonical request budget; Budget wraps this when set
	Context       context.Context
	Depth         int
	TargetRelease int
	SafeArchive   bool
}

// ArchiveBudget is a thin adapter over the canonical *workbudget.Budget.
// Nested archives and decompile must share the same Work() pointer.
type ArchiveBudget struct {
	work   *workbudget.Budget
	limits ArchiveLimits
}

func NewArchiveBudget(ctx context.Context, limits ArchiveLimits) *ArchiveBudget {
	return WrapWorkBudget(workbudget.New(ctx, limits.ToWorkLimits()), limits)
}

func WrapWorkBudget(b *workbudget.Budget, limits ArchiveLimits) *ArchiveBudget {
	if b == nil {
		return nil
	}
	return &ArchiveBudget{work: b, limits: limits}
}

func (b *ArchiveBudget) Work() *workbudget.Budget {
	if b == nil {
		return nil
	}
	return b.work
}

func (b *ArchiveBudget) Context() context.Context {
	if b == nil {
		return nil
	}
	return b.work.Context()
}

func (b *ArchiveBudget) Err() error {
	if b == nil {
		return nil
	}
	return b.work.Err()
}

func (b *ArchiveBudget) Used(name string) int64 {
	if b == nil || b.work == nil {
		return 0
	}
	return b.work.Used(workbudget.Counter(name))
}

func (b *ArchiveBudget) Snapshot() map[string]int64 {
	if b == nil || b.work == nil {
		return nil
	}
	src := b.work.Snapshot()
	out := make(map[string]int64, len(src))
	for k, v := range src {
		out[string(k)] = v
	}
	return out
}

func (b *ArchiveBudget) Limits() ArchiveLimits {
	if b == nil {
		return ArchiveLimits{}
	}
	return b.limits
}

// Charge checks cancel, then the named counter, before the caller allocates.
// archive_entry_bytes is a per-entry check (does not accumulate into total).
// archive_depth is a nesting gauge. entries/total accumulate and bill request_work.
func (b *ArchiveBudget) Charge(name string, n int64) error {
	if b == nil || b.work == nil {
		return nil
	}
	switch name {
	case CounterArchiveEntryBytes:
		return b.work.CheckArchiveEntryBytes(n)
	case CounterArchiveDepth:
		return b.work.CheckDepth(workbudget.CounterArchiveDepth, n)
	case CounterArchiveEntries:
		return b.work.Charge(workbudget.CounterArchiveEntries, n)
	case CounterArchiveTotalBytes:
		return b.work.Charge(workbudget.CounterArchiveTotalBytes, n)
	default:
		return b.work.Charge(workbudget.Counter(name), n)
	}
}

// ArchiveError is the structured archive failure (resource_limit / canceled / invalid_input).
type ArchiveError struct {
	Kind    string
	Counter string
	Used    int64
	Limit   int64
	Code    string
	Err     error
}

func (e *ArchiveError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	if e.Kind != "" {
		b.WriteString(e.Kind)
	}
	if e.Code != "" {
		if b.Len() > 0 {
			b.WriteByte(':')
		}
		b.WriteString(e.Code)
	}
	if e.Counter != "" {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(e.Counter)
	}
	if e.Limit != 0 || e.Used != 0 {
		fmt.Fprintf(&b, " used=%d limit=%d", e.Used, e.Limit)
	}
	if e.Err != nil {
		if b.Len() > 0 {
			b.WriteString(": ")
		}
		b.WriteString(e.Err.Error())
	}
	if b.Len() == 0 {
		return KindInvalid
	}
	return b.String()
}

func (e *ArchiveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func resourceLimitError(counter string, used, limit int64) error {
	return &ArchiveError{Kind: KindResource, Counter: counter, Used: used, Limit: limit}
}

func canceledError(err error) error {
	return &ArchiveError{Kind: KindCanceled, Err: err}
}

func pathRejectedError(reason, name string) error {
	msg := reason
	if name != "" {
		msg = reason + ": " + name
	}
	return &ArchiveError{
		Kind: KindInvalid,
		Code: CodeArchivePathRejected,
		Err:  errors.New(msg),
	}
}

func IsResourceLimit(err error) bool {
	var e *ArchiveError
	if errors.As(err, &e) && e.Kind == KindResource {
		return true
	}
	return err != nil && strings.Contains(err.Error(), KindResource)
}

func IsPathRejected(err error) bool {
	var e *ArchiveError
	if errors.As(err, &e) && (e.Kind == KindInvalid || e.Code == CodeArchivePathRejected) {
		return true
	}
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, KindInvalid) || strings.Contains(s, CodeArchivePathRejected)
}

func IsCanceled(err error) bool {
	var e *ArchiveError
	if errors.As(err, &e) && e.Kind == KindCanceled {
		return true
	}
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), KindCanceled))
}

func WithArchiveLimits(l ArchiveLimits) ZipFSOption {
	return func(c *zipFSConfig) {
		c.limits = l
		c.limitsSet = true
		c.safeArchive = true
	}
}

func WithArchiveBudget(b *ArchiveBudget) ZipFSOption {
	return func(c *zipFSConfig) {
		c.budget = b
		if b != nil {
			c.work = b.Work()
			c.safeArchive = true
		}
	}
}

func WithWorkBudget(b *workbudget.Budget) ZipFSOption {
	return func(c *zipFSConfig) {
		c.work = b
		if b != nil {
			c.safeArchive = true
		}
	}
}

func WithArchiveContext(ctx context.Context) ZipFSOption {
	return func(c *zipFSConfig) {
		c.ctx = ctx
	}
}

func WithArchiveDepth(depth int) ZipFSOption {
	return func(c *zipFSConfig) {
		c.depth = depth
	}
}

func WithTargetRelease(n int) ZipFSOption {
	return func(c *zipFSConfig) {
		c.targetRelease = n
	}
}

func WithSafeArchive(v bool) ZipFSOption {
	return func(c *zipFSConfig) {
		c.safeArchive = v
	}
}

func WithArchiveOptions(o ArchiveOptions) ZipFSOption {
	return func(c *zipFSConfig) {
		c.limits = o.Limits
		c.limitsSet = true
		c.budget = o.Budget
		c.work = o.Work
		if o.Work == nil && o.Budget != nil {
			c.work = o.Budget.Work()
		}
		c.ctx = o.Context
		c.depth = o.Depth
		c.targetRelease = o.TargetRelease
		if o.SafeArchive || o.Budget != nil || o.Work != nil || o.Limits.limited() || o.Context != nil || o.TargetRelease != 0 || o.Depth != 0 {
			c.safeArchive = true
		}
		if o.SafeArchive {
			c.safeArchive = true
		}
	}
}

// NewZipFSRawWithLimits constructs a ZipFS with catalog/read budgets.
// Existing NewZipFS* signatures stay unlimited.
func NewZipFSRawWithLimits(i io.ReaderAt, size int64, limits ArchiveLimits, budget *ArchiveBudget) (*ZipFS, error) {
	return NewZipFSRawWithOptions(i, size,
		WithArchiveLimits(limits),
		WithArchiveBudget(budget),
		WithSafeArchive(true),
	)
}

func NewZipFSFromStringWithLimits(i string, limits ArchiveLimits, budget *ArchiveBudget) (*ZipFS, error) {
	raw := []byte(i)
	mf := memfile.New(raw)
	return NewZipFSRawWithLimits(mf, int64(len(raw)), limits, budget)
}

func (z *ZipFS) ArchiveBudget() *ArchiveBudget {
	if z == nil {
		return nil
	}
	return z.budget
}

func (z *ZipFS) ArchiveLimits() ArchiveLimits {
	if z == nil {
		return ArchiveLimits{}
	}
	return z.limits
}

func (z *ZipFS) ArchiveContext() context.Context {
	if z == nil {
		return nil
	}
	return z.ctx
}

func (z *ZipFS) ArchiveDepth() int {
	if z == nil {
		return 0
	}
	return z.depth
}

func (z *ZipFS) TargetRelease() int {
	if z == nil {
		return 0
	}
	return z.targetRelease
}

func (z *ZipFS) SafeArchive() bool {
	if z == nil {
		return false
	}
	return z.safeArchive
}

func (z *ZipFS) ArchiveActive() bool {
	if z == nil {
		return false
	}
	return z.archiveActive
}

func (z *ZipFS) EntryNames() []string {
	if z == nil || z.r == nil {
		return nil
	}
	names := make([]string, 0, len(z.r.File))
	for _, f := range z.r.File {
		if f != nil {
			names = append(names, f.Name)
		}
	}
	return names
}

// NestedArchiveOptions copies this ZipFS's budget/limits/release onto a nested archive at depth.
func (z *ZipFS) NestedArchiveOptions(depth int) []ZipFSOption {
	if z == nil || !z.archiveActive {
		return nil
	}
	opts := []ZipFSOption{
		WithArchiveLimits(z.limits),
		WithArchiveBudget(z.budget),
		WithArchiveDepth(depth),
		WithTargetRelease(z.targetRelease),
		WithSafeArchive(true),
	}
	if z.ctx != nil {
		opts = append(opts, WithArchiveContext(z.ctx))
	}
	if z.password != "" {
		opts = append(opts, WithZipFSPassword(z.password))
	}
	return opts
}

func (z *ZipFS) applyArchiveConfig(cfg *zipFSConfig) error {
	if z == nil || cfg == nil {
		return nil
	}
	z.limits = cfg.limits
	z.ctx = cfg.ctx
	z.depth = cfg.depth
	z.targetRelease = cfg.targetRelease
	z.safeArchive = cfg.safeArchive
	z.budget = cfg.budget
	z.archiveActive = cfg.limitsSet || cfg.budget != nil || cfg.work != nil || cfg.safeArchive || cfg.ctx != nil || cfg.targetRelease != 0 || cfg.depth != 0
	if err := z.limits.Validate(); err != nil {
		return err
	}
	if cfg.work != nil {
		z.budget = WrapWorkBudget(cfg.work, z.limits)
	}
	if z.budget == nil && z.limits.limited() {
		z.budget = NewArchiveBudget(z.ctx, z.limits)
	}
	if z.budget != nil && z.ctx == nil {
		z.ctx = z.budget.Context()
	}
	if z.depth > 0 {
		if z.budget != nil {
			if err := z.budget.Charge(CounterArchiveDepth, int64(z.depth)); err != nil {
				return err
			}
		} else if z.limits.MaxArchiveDepth > 0 && z.depth > z.limits.MaxArchiveDepth {
			return resourceLimitError(CounterArchiveDepth, int64(z.depth), int64(z.limits.MaxArchiveDepth))
		}
	}
	return nil
}

func (z *ZipFS) catalogArchiveFiles() error {
	if z == nil || z.r == nil {
		return nil
	}
	strict := z.safeArchive || z.archiveActive
	seen := make(map[string]struct{})
	seenFold := make(map[string]string)
	seenFile := make(map[string]struct{})
	seenDir := make(map[string]struct{})

	for _, f := range z.r.File {
		if f == nil {
			continue
		}
		if z.budget != nil {
			if err := z.budget.Charge(CounterArchiveEntries, 1); err != nil {
				return err
			}
		} else if z.limits.MaxArchiveEntries > 0 {
			return resourceLimitError(CounterArchiveEntries, 0, z.limits.MaxArchiveEntries)
		}

		rawName := f.Name
		var forestName string
		if strict {
			norm, err := NormalizeArchiveEntryName(rawName)
			if err != nil {
				return err
			}
			if norm == "" || norm == "." {
				continue
			}
			isDir := archiveEntryIsDir(f, rawName)
			key := strings.TrimSuffix(norm, "/")
			if _, ok := seen[key]; ok {
				return pathRejectedError("duplicate entry", key)
			}
			seen[key] = struct{}{}
			fold := strings.ToLower(key)
			if prev, ok := seenFold[fold]; ok && prev != key {
				return pathRejectedError("case-fold collision", prev+" vs "+key)
			}
			seenFold[fold] = key
			if err := noteFileDirCollision(seenFile, seenDir, key, isDir); err != nil {
				return err
			}
			forestName = zipPathClean(key)
		} else {
			forestName = zipPathClean(rawName)
		}
		if err := z.forest.AddPath(forestName, f); err != nil {
			if strict {
				return pathRejectedError("path forest", rawName)
			}
			log.Warnf("BUG: cache zip tree failed: %v", err)
			continue
		}
	}
	return nil
}

func archiveEntryIsDir(f *zip.File, rawName string) bool {
	if f != nil && f.FileInfo().IsDir() {
		return true
	}
	n := strings.ReplaceAll(rawName, "\\", "/")
	return strings.HasSuffix(n, "/")
}

func noteFileDirCollision(files, dirs map[string]struct{}, key string, isDir bool) error {
	if isDir {
		if _, ok := files[key]; ok {
			return pathRejectedError("file/directory collision", key)
		}
		dirs[key] = struct{}{}
	} else {
		if _, ok := dirs[key]; ok {
			return pathRejectedError("file/directory collision", key)
		}
		files[key] = struct{}{}
	}
	for p := path.Dir(key); p != "." && p != "/" && p != ""; p = path.Dir(p) {
		if _, ok := files[p]; ok {
			return pathRejectedError("file/directory collision", p)
		}
		dirs[p] = struct{}{}
	}
	return nil
}

// NormalizeArchiveEntryName applies the strict archive path policy:
// backslash→slash, path.Clean, reject NUL, absolute paths, and zip-slip `..`.
func NormalizeArchiveEntryName(name string) (string, error) {
	if strings.IndexByte(name, 0) >= 0 {
		return "", pathRejectedError("NUL in path", name)
	}
	n := strings.ReplaceAll(name, "\\", "/")
	n = strings.TrimSpace(n)
	if n == "" || n == "." {
		return ".", nil
	}
	if strings.HasPrefix(n, "/") || strings.HasPrefix(n, "//") {
		return "", pathRejectedError("absolute path", name)
	}
	if len(n) >= 2 && n[1] == ':' && isDriveLetter(n[0]) {
		return "", pathRejectedError("absolute path", name)
	}
	cleaned := path.Clean(n)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ".", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", pathRejectedError("zip-slip", name)
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return "", pathRejectedError("zip-slip", name)
		}
		if strings.IndexByte(part, 0) >= 0 {
			return "", pathRejectedError("NUL in path", name)
		}
	}
	return cleaned, nil
}

func isDriveLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'Z')
}

const multiReleasePrefix = "META-INF/versions/"

// SelectMultiReleasePath implements JEP 238 selection.
// targetRelease==0: base classPath only.
// targetRelease>=9: highest META-INF/versions/M/classPath with M<=targetRelease, else base.
func SelectMultiReleasePath(entries []string, classPath string, targetRelease int) (chosen string, ok bool) {
	classPath = normalizeClassPath(classPath)
	if classPath == "" {
		return "", false
	}
	if strings.HasPrefix(classPath, multiReleasePrefix) {
		for _, raw := range entries {
			if normalizeClassPath(raw) == classPath {
				return classPath, true
			}
		}
		return "", false
	}

	basePresent := false
	bestM := -1
	best := ""
	for _, raw := range entries {
		e := normalizeClassPath(raw)
		if e == "" {
			continue
		}
		if e == classPath {
			basePresent = true
			continue
		}
		rest, found := strings.CutPrefix(e, multiReleasePrefix)
		if !found {
			continue
		}
		slash := strings.IndexByte(rest, '/')
		if slash <= 0 {
			continue
		}
		m, err := strconv.Atoi(rest[:slash])
		if err != nil || m < 9 {
			continue
		}
		if rest[slash+1:] != classPath {
			continue
		}
		if targetRelease >= 9 && m <= targetRelease && m >= bestM {
			bestM = m
			best = e
		}
	}
	if targetRelease < 9 {
		if basePresent {
			return classPath, true
		}
		return "", false
	}
	if bestM >= 9 {
		return best, true
	}
	if basePresent {
		return classPath, true
	}
	return "", false
}

func normalizeClassPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	if p == "." || p == "" {
		return ""
	}
	return p
}

// ArchiveCacheKey includes TargetRelease so jar/zip caches cannot mix releases.
func ArchiveCacheKey(archivePath string, targetRelease int) string {
	return strconv.Itoa(targetRelease) + "\x1e" + archivePath
}

func (z *ZipFS) resolveReadName(name string) string {
	if z == nil {
		return name
	}
	cleaned := z.Clean(name)
	if z.targetRelease == 0 || z.r == nil {
		return cleaned
	}
	logical := strings.TrimPrefix(cleaned, "./")
	if strings.HasPrefix(logical, multiReleasePrefix) {
		return cleaned
	}
	if chosen, ok := SelectMultiReleasePath(z.EntryNames(), logical, z.targetRelease); ok {
		return z.Clean(chosen)
	}
	return cleaned
}

func (z *ZipFS) readFileBounded(name string) ([]byte, error) {
	name = z.resolveReadName(name)
	node, err := z.forest.Get(name)
	if err != nil {
		return nil, utils.Wrapf(err, "get %v failed", name)
	}
	if node == nil || utils.IsNil(node.Value) {
		return nil, os.ErrNotExist
	}
	f, ok := node.Value.(*zip.File)
	if !ok {
		return nil, os.ErrNotExist
	}
	if f.FileInfo().IsDir() {
		return nil, utils.Wrapf(os.ErrNotExist, "%v is dir", name)
	}
	if f.IsEncrypted() {
		if z.password == "" {
			return nil, utils.Errorf("zip entry %s is encrypted but no password supplied", f.Name)
		}
		f.SetPassword(z.password)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	ctx := z.ctx
	if ctx == nil && z.budget != nil {
		ctx = z.budget.Context()
	}
	return ReadArchiveStream(ctx, rc, f.UncompressedSize64, z.budget, z.limits)
}

// ReadArchiveStream reads decompressed bytes from r.
// headerUncompressed may early-reject when greater than MaxArchiveEntryBytes,
// but a lying-small header is still caught by counting actual bytes.
// Never preallocates from the untrusted header size.
func ReadArchiveStream(ctx context.Context, r io.Reader, headerUncompressed uint64, budget *ArchiveBudget, limits ArchiveLimits) ([]byte, error) {
	if r == nil {
		return nil, errors.New("nil archive reader")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, canceledError(err)
		}
	} else if budget != nil {
		ctx = budget.Context()
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, canceledError(err)
			}
		}
	}
	maxEntry := limits.MaxArchiveEntryBytes
	if maxEntry > 0 {
		if headerUncompressed > uint64(math.MaxInt64) || headerUncompressed > uint64(maxEntry) {
			used := maxEntry
			if budget != nil {
				_ = budget.Charge(CounterArchiveEntryBytes, used)
			}
			return nil, resourceLimitError(CounterArchiveEntryBytes, used, maxEntry)
		}
	}

	src := r
	if ctx != nil {
		src = &ctxReader{ctx: ctx, r: src}
	}
	// LimitReader(n+1) must not wrap: MaxInt64+1 is negative and yields empty success.
	if maxEntry > 0 && maxEntry < math.MaxInt64 {
		src = io.LimitReader(src, maxEntry+1)
	}

	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	var running int64
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, canceledError(err)
			}
		}
		n, err := src.Read(chunk)
		if n > 0 {
			add := int64(n)
			if running > math.MaxInt64-add {
				return nil, resourceLimitError(CounterArchiveEntryBytes, running, maxEntry)
			}
			running += add
			if maxEntry > 0 && running > maxEntry {
				if budget != nil {
					_ = budget.Charge(CounterArchiveEntryBytes, running)
				}
				return nil, resourceLimitError(CounterArchiveEntryBytes, running, maxEntry)
			}
			if budget != nil {
				if err := budget.Charge(CounterArchiveEntryBytes, running); err != nil {
					return nil, err
				}
				if err := budget.Charge(CounterArchiveTotalBytes, add); err != nil {
					return nil, err
				}
			} else if limits.MaxArchiveTotalBytes > 0 && running > limits.MaxArchiveTotalBytes {
				return nil, resourceLimitError(CounterArchiveTotalBytes, running, limits.MaxArchiveTotalBytes)
			}
			if _, werr := buf.Write(chunk[:n]); werr != nil {
				return nil, werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, canceledError(ctx.Err())
			}
			if errors.Is(err, zip.ErrChecksum) {
				return nil, err
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, err
			}
			var ae *ArchiveError
			if errors.As(err, &ae) {
				return nil, err
			}
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if c.ctx != nil {
		if err := c.ctx.Err(); err != nil {
			return 0, canceledError(err)
		}
	}
	n, err := c.r.Read(p)
	if c.ctx != nil {
		if err2 := c.ctx.Err(); err2 != nil {
			return n, canceledError(err2)
		}
	}
	return n, err
}
