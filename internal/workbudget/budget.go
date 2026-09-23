package workbudget

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Counter names match the T22/T23 production contract.
type Counter string

const (
	CounterInputBytes        Counter = "input_bytes"
	CounterReadBytes         Counter = "read_bytes"
	CounterReadOps           Counter = "read_ops"
	CounterParseItems        Counter = "parse_items"
	CounterGraphScans        Counter = "graph_scans"
	CounterGraphEdges        Counter = "graph_edges"
	CounterSetElementWork    Counter = "set_element_work"
	CounterAnalysisUpdates   Counter = "analysis_updates"
	CounterRequestWork       Counter = "request_work"
	CounterOutputBytes       Counter = "output_bytes"
	CounterDiagnostics       Counter = "diagnostics"
	CounterASTDepth          Counter = "ast_depth"
	CounterArchiveEntries    Counter = "archive_entries"
	CounterArchiveEntryBytes Counter = "archive_entry_bytes"
	CounterArchiveTotalBytes Counter = "archive_total_bytes"
	CounterArchiveDepth      Counter = "archive_depth"
	CounterNodeCopies        Counter = "node_copies"
	// CounterIntermediateBytes is the high-water of one construction allocation
	// (not retained source). Cap is derived from MaxOutputBytes (CheckAlloc).
	// It does not add to RequestWork.
	CounterIntermediateBytes Counter = "intermediate_bytes"
)

const intermediateOutputFactor int64 = 8

func intermediateLimit(l Limits) int64 {
	maxOut := l.MaxOutputBytes
	if maxOut <= 0 {
		return 0
	}
	if maxOut > math.MaxInt64/intermediateOutputFactor {
		return math.MaxInt64
	}
	return maxOut * intermediateOutputFactor
}

const (
	KindBudget   = "analysis_budget_exceeded"
	KindResource = "resource_limit"
	KindCanceled = "canceled"
)

// Limits are request-scoped. Zero fields are unlimited; negative fields are invalid.
type Limits struct {
	MaxInputBytes        int64
	MaxReadBytes         int64
	MaxParseItems        int64
	MaxGraphScans        int64
	MaxGraphEdges        int64
	MaxSetElementWork    int64
	MaxRequestWork       int64
	MaxOutputBytes       int64
	MaxASTDepth          int
	MaxDiagnostics       int
	MaxArchiveEntries    int64
	MaxArchiveEntryBytes int64
	MaxArchiveTotalBytes int64
	MaxArchiveDepth      int
	MaxNodeCopies        int64
}

// Validate reports a config error for any negative field. The message must not
// claim the class bytes are invalid.
func (l Limits) Validate() error {
	v := reflect.ValueOf(l)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Int, reflect.Int64:
			if f.Int() < 0 {
				return fmt.Errorf("negative limit %s", t.Field(i).Name)
			}
		}
	}
	return nil
}

func (l Limits) limitFor(c Counter) int64 {
	switch c {
	case CounterInputBytes:
		return l.MaxInputBytes
	case CounterReadBytes:
		return l.MaxReadBytes
	case CounterParseItems:
		return l.MaxParseItems
	case CounterGraphScans:
		return l.MaxGraphScans
	case CounterGraphEdges:
		return l.MaxGraphEdges
	case CounterSetElementWork:
		return l.MaxSetElementWork
	case CounterAnalysisUpdates:
		return 0
	case CounterRequestWork:
		return l.MaxRequestWork
	case CounterOutputBytes:
		return l.MaxOutputBytes
	case CounterDiagnostics:
		return int64(l.MaxDiagnostics)
	case CounterASTDepth:
		return int64(l.MaxASTDepth)
	case CounterArchiveEntries:
		return l.MaxArchiveEntries
	case CounterArchiveEntryBytes:
		return l.MaxArchiveEntryBytes
	case CounterArchiveTotalBytes:
		return l.MaxArchiveTotalBytes
	case CounterArchiveDepth:
		return int64(l.MaxArchiveDepth)
	case CounterNodeCopies:
		return l.MaxNodeCopies
	default:
		return 0
	}
}

// Error is the structured budget/cancel failure.
type Error struct {
	Kind    string
	Counter Counter
	Used    int64
	Limit   int64
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: counter=%s used=%d limit=%d: %v", e.Kind, e.Counter, e.Used, e.Limit, e.Err)
	}
	return fmt.Sprintf("%s: counter=%s used=%d limit=%d", e.Kind, e.Counter, e.Used, e.Limit)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is reports whether err is or wraps a budget Error.
func Is(err error) bool {
	var e *Error
	return errors.As(err, &e)
}

// Budget is request-scoped, concurrency-safe, and not reset per method.
type Budget struct {
	mu      sync.Mutex
	ctx     context.Context
	limits  Limits
	used    map[Counter]int64
	nesting map[Counter]int64
	err     error
}

func New(ctx context.Context, limits Limits) *Budget {
	return &Budget{
		ctx:     ctx,
		limits:  limits,
		used:    map[Counter]int64{},
		nesting: map[Counter]int64{},
	}
}

func (b *Budget) Context() context.Context {
	if b == nil {
		return nil
	}
	return b.ctx
}

func (b *Budget) Err() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *Budget) Used(c Counter) int64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used[c]
}

func (b *Budget) Snapshot() map[Counter]int64 {
	if b == nil {
		return map[Counter]int64{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[Counter]int64, len(b.used))
	for k, v := range b.used {
		out[k] = v
	}
	return out
}

func (b *Budget) checkCancelLocked() error {
	if b.err != nil {
		return b.err
	}
	if b.ctx == nil {
		return nil
	}
	if err := b.ctx.Err(); err != nil {
		fail := &Error{Kind: KindCanceled, Err: err}
		if b.err == nil {
			b.err = fail
		}
		return fail
	}
	return nil
}

// CheckContext also honors an explicit request context when callers supply a
// preexisting archive budget. The first error remains shared and sticky.
func (b *Budget) CheckContext(ctx context.Context) error {
	if b == nil {
		if ctx != nil {
			return ctx.Err()
		}
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		b.err = &Error{Kind: KindCanceled, Err: ctx.Err()}
		return b.err
	}
	return nil
}

func overflowOrExceed(used, n, limit int64) bool {
	if n <= 0 {
		return false
	}
	if used > math.MaxInt64-n {
		return true
	}
	if limit <= 0 {
		return false
	}
	return used+n > limit
}

func kindFor(c Counter) string {
	if c == CounterAnalysisUpdates {
		return KindBudget
	}
	return KindResource
}

func exceedError(c Counter, used, n, limit int64) *Error {
	reported := limit
	if limit <= 0 || used > math.MaxInt64-n {
		reported = math.MaxInt64
		if limit > 0 {
			reported = limit
		}
	}
	return &Error{Kind: kindFor(c), Counter: c, Used: used, Limit: reported}
}

// Charge checks cancel, then (used+n) against the counter limit, then adds n to
// RequestWork (except when c is already RequestWork), all before the caller
// allocates. n<=0 is a cancel check only. Nil Budget is a no-op success.
func (b *Budget) Charge(c Counter, n int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if n <= 0 {
		return nil
	}
	return b.chargeLocked(c, n, true)
}

// ChargeMany validates every named counter and aggregate request work before
// committing any counter. Sorted names make simultaneous failures deterministic.
// Nonpositive entries retain Charge's cancellation-check-only semantics.
func (b *Budget) ChargeMany(cost map[Counter]int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	return b.chargeManyLocked(cost, true)
}

func (b *Budget) chargeLocked(c Counter, n int64, aggregate bool) error {
	return b.chargeManyLocked(map[Counter]int64{c: n}, aggregate)
}

func (b *Budget) chargeManyLocked(cost map[Counter]int64, aggregate bool) error {
	keys := make([]Counter, 0, len(cost))
	for c, n := range cost {
		if n > 0 {
			keys = append(keys, c)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	total := int64(0)
	for _, c := range keys {
		n := cost[c]
		if overflowOrExceed(b.used[c], n, b.limits.limitFor(c)) {
			b.err = exceedError(c, b.used[c], n, b.limits.limitFor(c))
			return b.err
		}
		if aggregate {
			if total > math.MaxInt64-n {
				b.err = exceedError(CounterRequestWork, total, n, b.limits.MaxRequestWork)
				return b.err
			}
			total += n
		}
	}
	if aggregate && overflowOrExceed(b.used[CounterRequestWork], total, b.limits.MaxRequestWork) {
		b.err = exceedError(CounterRequestWork, b.used[CounterRequestWork], total, b.limits.MaxRequestWork)
		return b.err
	}
	for _, c := range keys {
		if !aggregate || c != CounterRequestWork {
			b.used[c] += cost[c]
		}
	}
	if aggregate {
		b.used[CounterRequestWork] += total
	}
	return nil
}

// CheckedProduct rejects unrepresentable allocation sizes before multiplication.
func CheckedProduct(count, width int64) (int64, error) {
	if count < 0 || width < 0 || width != 0 && count > math.MaxInt64/width {
		return 0, fmt.Errorf("resource_limit: allocation size overflow")
	}
	return count * width, nil
}

// CheckOutput reports whether retaining `total` emitted source bytes would
// exceed MaxOutputBytes. It does not increment counters.
func (b *Budget) CheckOutput(total int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	if total < 0 {
		total = 0
	}
	limit := b.limits.MaxOutputBytes
	if overflowOrExceed(0, total, limit) {
		fail := exceedError(CounterOutputBytes, 0, total, limit)
		fail.Used = total
		if b.err == nil {
			b.err = fail
		}
		return fail
	}
	return nil
}

// EnsureOutput sets the high-water of retained emitted source to at least
// `total`. RequestWork is charged only for the delta above the previous
// high-water, so concatenating already-rendered fragments cannot multiply-count
// the same bytes.
func (b *Budget) EnsureOutput(total int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	if total < 0 {
		total = 0
	}
	used := b.used[CounterOutputBytes]
	if total <= used {
		return nil
	}
	return b.chargeLocked(CounterOutputBytes, total-used, true)
}

// CheckAlloc records the high-water of one construction allocation against the
// derived intermediate cap (8 * MaxOutputBytes when MaxOutputBytes > 0).
func (b *Budget) CheckAlloc(n int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	if n <= 0 {
		return nil
	}
	limit := intermediateLimit(b.limits)
	if overflowOrExceed(0, n, limit) {
		fail := exceedError(CounterIntermediateBytes, 0, n, limit)
		fail.Used = n
		if b.err == nil {
			b.err = fail
		}
		return fail
	}
	if n > b.used[CounterIntermediateBytes] {
		b.used[CounterIntermediateBytes] = n
	}
	return nil
}

// RemainingOutput is MaxOutputBytes minus the retained high-water.
// RenderGuarded reports whether value rendering must Check/Enter depth and
// preflight output. Unlimited default dumps must not pay this on every node.
func (b *Budget) RenderGuarded() bool {
	if b == nil {
		return false
	}
	if b.limits.MaxOutputBytes > 0 || b.limits.MaxASTDepth > 0 {
		return true
	}
	if b.ctx != nil && b.ctx.Err() != nil {
		return true
	}
	return false
}

func (b *Budget) RemainingOutput() int64 {
	if b == nil {
		return math.MaxInt64
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := b.limits.MaxOutputBytes
	if limit <= 0 {
		return math.MaxInt64
	}
	used := b.used[CounterOutputBytes]
	if used >= limit {
		return 0
	}
	return limit - used
}

// HasOutputLimit reports whether rendering has an explicit output cap. It is
// used by legacy renderers that cannot prove an allocation bound before they
// run; zero keeps the public "unlimited" meaning.
func (b *Budget) HasOutputLimit() bool {
	return b != nil && b.limits.MaxOutputBytes > 0
}

// RejectUnboundedRender fails closed before an opaque renderer can allocate a
// complete string. Budget-aware renderers should use Writer instead. The
// reason is attached to the sticky resource error because no exact prospective
// byte count exists for an opaque callback.
func (b *Budget) RejectUnboundedRender() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	fail := &Error{
		Kind:    KindResource,
		Counter: CounterOutputBytes,
		Used:    b.used[CounterOutputBytes],
		Limit:   b.limits.MaxOutputBytes,
		Err:     errors.New("opaque renderer has no pre-allocation output bound"),
	}
	b.err = fail
	return fail
}

// FailRender makes a renderer's non-budget failure sticky and fail-closed.
// Writer operations normally set the budget error themselves; this covers a
// renderer that rejects an input without a Writer operation causing failure.
func (b *Budget) FailRender(err error) error {
	if b == nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if cancelErr := b.checkCancelLocked(); cancelErr != nil {
		return cancelErr
	}
	if b.err != nil {
		return b.err
	}
	if err == nil {
		err = errors.New("bounded renderer failed")
	}
	fail := &Error{
		Kind:    KindResource,
		Counter: CounterOutputBytes,
		Used:    b.used[CounterOutputBytes],
		Limit:   b.limits.MaxOutputBytes,
		Err:     fmt.Errorf("bounded renderer failed: %w", err),
	}
	b.err = fail
	return fail
}

// FailOutputOverflow records an unrepresentable retained-output size. It is
// sticky even when the configured output cap is unlimited: integer wraparound
// must never turn an impossible size into a successful budget check.
func (b *Budget) FailOutputOverflow() error {
	fail := &Error{
		Kind:    KindResource,
		Counter: CounterOutputBytes,
		Used:    math.MaxInt64,
		Limit:   0,
		Err:     errors.New("output byte count overflow"),
	}
	if b == nil {
		return fail
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if cancelErr := b.checkCancelLocked(); cancelErr != nil {
		return cancelErr
	}
	if b.err != nil {
		return b.err
	}
	fail.Limit = b.limits.MaxOutputBytes
	b.err = fail
	return fail
}

// Writer is a bounded strings.Builder. Each WriteString checks the derived
// intermediate cap and MaxOutputBytes (base + buffer length) before appending.
type Writer struct {
	buf  strings.Builder
	work *Budget
	base int64
}

func NewWriter(work *Budget) *Writer {
	return &Writer{work: work}
}

func (w *Writer) SetBase(held int64) {
	if w != nil {
		if held < 0 {
			held = 0
		}
		w.base = held
	}
}

func (w *Writer) WriteString(s string) error {
	if w == nil {
		return nil
	}
	n := int64(len(s))
	if err := w.work.CheckAlloc(n); err != nil {
		return err
	}
	current := int64(w.buf.Len())
	if current > math.MaxInt64-w.base || n > math.MaxInt64-w.base-current {
		return w.work.FailOutputOverflow()
	}
	next := w.base + current + n
	if err := w.work.CheckOutput(next); err != nil {
		return err
	}
	w.buf.WriteString(s)
	return nil
}

func (w *Writer) String() string {
	if w == nil {
		return ""
	}
	return w.buf.String()
}

func (w *Writer) Len() int {
	if w == nil {
		return 0
	}
	return w.buf.Len()
}

// Check reports cancellation (or a sticky prior error) without consuming units.
func (b *Budget) Check() error {
	return b.Charge(CounterRequestWork, 0)
}

// CheckDepth treats c as a nesting gauge (AST / archive depth), not a cumulative
// visit counter. It does not increment RequestWork.
func (b *Budget) CheckDepth(c Counter, depth int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	if depth < 0 {
		depth = 0
	}
	limit := b.limits.limitFor(c)
	if limit > 0 && depth > limit {
		fail := &Error{Kind: KindResource, Counter: c, Used: depth, Limit: limit}
		if b.err == nil {
			b.err = fail
		}
		return fail
	}
	if depth > b.used[c] {
		b.used[c] = depth
	}
	return nil
}

// Enter increases active nesting for a depth gauge. Leave must be paired.
func (b *Budget) Enter(c Counter) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	next := b.nesting[c] + 1
	limit := b.limits.limitFor(c)
	if limit > 0 && next > limit {
		fail := &Error{Kind: KindResource, Counter: c, Used: next, Limit: limit}
		if b.err == nil {
			b.err = fail
		}
		return fail
	}
	b.nesting[c] = next
	if next > b.used[c] {
		b.used[c] = next
	}
	return nil
}

func (b *Budget) Leave(c Counter) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.nesting[c] > 0 {
		b.nesting[c]--
	}
}

// CheckArchiveEntryBytes applies the per-entry cap to n without accumulating
// into archive_total_bytes or request_work. High-water of this counter is the
// largest single entry seen. Call Charge(CounterArchiveTotalBytes, chunk) for
// actual decompressed bytes so nested/decompile work share one aggregate.
func (b *Budget) CheckArchiveEntryBytes(n int64) error {
	if b == nil {
		return nil
	}
	if err := b.Check(); err != nil {
		return err
	}
	if n <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.checkCancelLocked(); err != nil {
		return err
	}
	if b.err != nil {
		return b.err
	}
	limit := b.limits.MaxArchiveEntryBytes
	if overflowOrExceed(0, n, limit) {
		fail := exceedError(CounterArchiveEntryBytes, 0, n, limit)
		fail.Used = n
		if b.err == nil {
			b.err = fail
		}
		return fail
	}
	if n > b.used[CounterArchiveEntryBytes] {
		b.used[CounterArchiveEntryBytes] = n
	}
	return nil
}

// ChargeArchiveEntryBytes is CheckArchiveEntryBytes plus charging n onto
// archive_total_bytes (and request_work). Use this when billing a finished
// entry as one shot. Stream readers must Check per running count and Charge
// total per chunk so bytes are not double-counted.
func (b *Budget) ChargeArchiveEntryBytes(n int64) error {
	if err := b.CheckArchiveEntryBytes(n); err != nil {
		return err
	}
	return b.Charge(CounterArchiveTotalBytes, n)
}
