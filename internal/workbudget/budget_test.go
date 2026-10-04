package workbudget

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestChargeNilAndNonPositive(t *testing.T) {
	var b *Budget
	if err := b.Charge(CounterGraphEdges, 1); err != nil {
		t.Fatal(err)
	}
	b = New(nil, Limits{MaxGraphEdges: 1})
	if err := b.Charge(CounterGraphEdges, 0); err != nil {
		t.Fatal(err)
	}
	if err := b.Charge(CounterGraphEdges, -3); err != nil {
		t.Fatal(err)
	}
	if b.Used(CounterGraphEdges) != 0 {
		t.Fatalf("non-positive charge mutated used: %d", b.Used(CounterGraphEdges))
	}
}

func TestChargeLimitAndRollback(t *testing.T) {
	b := New(context.Background(), Limits{MaxGraphEdges: 2, MaxRequestWork: 3})
	if err := b.Charge(CounterGraphEdges, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.Charge(CounterGraphEdges, 1); err == nil || !Is(err) {
		t.Fatalf("expected graph_edges reject, got %v", err)
	}
	if b.Used(CounterGraphEdges) != 2 || b.Used(CounterRequestWork) != 2 {
		t.Fatalf("failed charge mutated counters: %+v", b.Snapshot())
	}
	b = New(context.Background(), Limits{MaxGraphEdges: 10, MaxRequestWork: 2})
	if err := b.Charge(CounterGraphEdges, 2); err != nil {
		t.Fatal(err)
	}
	err := b.Charge(CounterGraphEdges, 1)
	if err == nil {
		t.Fatal("request_work should reject")
	}
	var e *Error
	if !errors.As(err, &e) || e.Counter != CounterRequestWork || e.Kind != KindResource {
		t.Fatalf("want request_work resource_limit, got %#v", e)
	}
	if b.Used(CounterGraphEdges) != 2 {
		t.Fatalf("specific counter not rolled back: %d", b.Used(CounterGraphEdges))
	}
}

func TestChargeOverflowDoesNotWrap(t *testing.T) {
	b := New(context.Background(), Limits{})
	b.mu.Lock()
	b.used[CounterGraphEdges] = math.MaxInt64 - 1
	b.used[CounterRequestWork] = math.MaxInt64 - 1
	b.mu.Unlock()
	if err := b.Charge(CounterGraphEdges, 2); err == nil {
		t.Fatal("overflow bypassed")
	}
	if b.Used(CounterGraphEdges) != math.MaxInt64-1 {
		t.Fatalf("overflow mutated used: %d", b.Used(CounterGraphEdges))
	}
	b = New(context.Background(), Limits{MaxGraphEdges: math.MaxInt64})
	if err := b.Charge(CounterGraphEdges, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := b.Charge(CounterGraphEdges, 1); err == nil {
		t.Fatal("MaxInt64 neighbor accepted")
	}
}

func TestDepthIsGaugeNotCumulative(t *testing.T) {
	b := New(context.Background(), Limits{MaxASTDepth: 1})
	if err := b.Enter(CounterASTDepth); err != nil {
		t.Fatal(err)
	}
	b.Leave(CounterASTDepth)
	if err := b.Enter(CounterASTDepth); err != nil {
		t.Fatal(err)
	}
	b.Leave(CounterASTDepth)
	deep := New(context.Background(), Limits{MaxASTDepth: 2})
	if err := deep.Enter(CounterASTDepth); err != nil {
		t.Fatal(err)
	}
	if err := deep.Enter(CounterASTDepth); err != nil {
		t.Fatal(err)
	}
	if err := deep.Enter(CounterASTDepth); err == nil {
		t.Fatal("depth 3 should fail")
	}
	if b.Used(CounterRequestWork) != 0 || deep.Used(CounterRequestWork) != 0 {
		t.Fatalf("depth charged request_work")
	}
	if deep.Used(CounterASTDepth) != 2 {
		t.Fatalf("high-water %d", deep.Used(CounterASTDepth))
	}
}

func TestArchiveEntryBytesArePerEntry(t *testing.T) {
	b := New(context.Background(), Limits{MaxArchiveEntryBytes: 1000, MaxArchiveTotalBytes: 10000})
	if err := b.ChargeArchiveEntryBytes(800); err != nil {
		t.Fatal(err)
	}
	if err := b.ChargeArchiveEntryBytes(800); err != nil {
		t.Fatal(err)
	}
	if b.Used(CounterArchiveTotalBytes) != 1600 {
		t.Fatalf("total %d", b.Used(CounterArchiveTotalBytes))
	}
	if err := b.ChargeArchiveEntryBytes(1001); err == nil {
		t.Fatal("per-entry cap bypassed")
	}
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := New(ctx, Limits{MaxGraphEdges: 100})
	cancel()
	err := b.Charge(CounterGraphEdges, 1)
	if err == nil || !Is(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	var e *Error
	errors.As(err, &e)
	if e.Kind != KindCanceled {
		t.Fatalf("kind %s", e.Kind)
	}
}

func TestValidateNegative(t *testing.T) {
	if err := (Limits{MaxOutputBytes: -1}).Validate(); err == nil {
		t.Fatal("expected negative limit")
	}
	if err := (Limits{}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAnalysisUpdatesKind(t *testing.T) {
	b := New(context.Background(), Limits{})
	b.mu.Lock()
	b.used[CounterAnalysisUpdates] = math.MaxInt64
	b.mu.Unlock()
	err := b.Charge(CounterAnalysisUpdates, 1)
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindBudget {
		t.Fatalf("analysis_updates kind: %v", err)
	}
}

func TestEnsureOutputHighWaterNoDoubleCount(t *testing.T) {
	const L int64 = 100
	b := New(context.Background(), Limits{MaxOutputBytes: L})
	if err := b.EnsureOutput(L); err != nil {
		t.Fatal(err)
	}
	if err := b.EnsureOutput(L); err != nil {
		t.Fatal("second EnsureOutput(L) must not double-count")
	}
	if b.Used(CounterOutputBytes) != L {
		t.Fatalf("used=%d want %d", b.Used(CounterOutputBytes), L)
	}
	if b.Used(CounterRequestWork) != L {
		t.Fatalf("request_work=%d want %d", b.Used(CounterRequestWork), L)
	}
	if err := b.EnsureOutput(L + 1); err == nil {
		t.Fatal("L+1 should fail")
	}
}

func TestCheckOutputLpm1(t *testing.T) {
	const L int64 = 50
	for _, n := range []int64{L - 1, L} {
		b := New(context.Background(), Limits{MaxOutputBytes: L})
		if err := b.CheckOutput(n); err != nil {
			t.Fatalf("CheckOutput(%d): %v", n, err)
		}
		if err := b.EnsureOutput(n); err != nil {
			t.Fatalf("EnsureOutput(%d): %v", n, err)
		}
	}
	b := New(context.Background(), Limits{MaxOutputBytes: L})
	if err := b.CheckOutput(L + 1); err == nil {
		t.Fatal("CheckOutput(L+1) succeeded")
	}
	if b.Used(CounterOutputBytes) != 0 {
		t.Fatalf("failed CheckOutput mutated used: %d", b.Used(CounterOutputBytes))
	}
}

func TestCheckAllocDerivedFromMaxOutputBytes(t *testing.T) {
	b := New(context.Background(), Limits{MaxOutputBytes: 10})
	if err := b.CheckAlloc(80); err != nil {
		t.Fatalf("8*MaxOutputBytes must be allowed: %v", err)
	}
	if err := b.CheckAlloc(81); err == nil {
		t.Fatal("intermediate over 8*MaxOutputBytes accepted")
	}
	var e *Error
	if !errors.As(b.Err(), &e) || e.Counter != CounterIntermediateBytes {
		t.Fatalf("want intermediate_bytes, got %#v", e)
	}
	if b.Used(CounterRequestWork) != 0 {
		t.Fatalf("CheckAlloc charged request_work: %d", b.Used(CounterRequestWork))
	}
	unlimited := New(context.Background(), Limits{})
	if err := unlimited.CheckAlloc(1 << 20); err != nil {
		t.Fatal(err)
	}
}

func TestWriterBoundsRetainedOutput(t *testing.T) {
	b := New(context.Background(), Limits{MaxOutputBytes: 5})
	w := NewWriter(b)
	if err := w.WriteString("abcd"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteString("ef"); err == nil {
		t.Fatal("writer exceeded MaxOutputBytes")
	}
	if w.String() != "abcd" {
		t.Fatalf("partial keep: %q", w.String())
	}
}

func TestWriterBaseOverflowFailsClosedBeforeAppend(t *testing.T) {
	b := New(context.Background(), Limits{})
	w := NewWriter(b)
	w.SetBase(math.MaxInt64 - 1)
	if err := w.WriteString("xx"); err == nil || !Is(err) {
		t.Fatalf("output count overflow accepted: %v", err)
	}
	if w.Len() != 0 {
		t.Fatalf("overflow appended bytes before rejecting: %q", w.String())
	}
	var be *Error
	if !errors.As(b.Err(), &be) || be.Counter != CounterOutputBytes || !strings.Contains(be.Error(), "overflow") {
		t.Fatalf("missing sticky output overflow evidence: %v", b.Err())
	}
}
