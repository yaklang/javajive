package workbudget

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestRequestChargeManyAtomicAndDeterministic(t *testing.T) {
	for i := 0; i < 100; i++ {
		b := New(nil, Limits{MaxGraphEdges: 1, MaxGraphScans: 1})
		err := b.ChargeMany(map[Counter]int64{CounterGraphScans: 2, CounterGraphEdges: 2})
		var be *Error
		if !errors.As(err, &be) || be.Counter != CounterGraphEdges {
			t.Fatalf("unstable failure: %v", err)
		}
		if len(b.Snapshot()) != 0 {
			t.Fatal("failed transaction mutated counters")
		}
	}
	b := New(nil, Limits{MaxRequestWork: 3})
	if err := b.ChargeMany(map[Counter]int64{CounterReadOps: 1, CounterReadBytes: 2}); err != nil {
		t.Fatal(err)
	}
	before := b.Snapshot()
	if err := b.ChargeMany(map[Counter]int64{CounterReadOps: 1, CounterReadBytes: 1}); err == nil {
		t.Fatal("aggregate cap bypassed")
	}
	if !reflect.DeepEqual(before, b.Snapshot()) {
		t.Fatal("aggregate failure partially charged")
	}
}

func TestRequestFirstFailureRemainsSticky(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := New(ctx, Limits{MaxInputBytes: 1})
	first := b.Charge(CounterInputBytes, 2)
	if first == nil {
		t.Fatal("missing failure")
	}
	cancel()
	for _, fn := range []func() error{b.Check, func() error { return b.CheckContext(ctx) }, func() error { return b.ChargeMany(map[Counter]int64{CounterReadOps: 1}) }, func() error { return b.CheckAlloc(1) }, func() error { return b.EnsureOutput(1) }, func() error { return b.Enter(CounterASTDepth) }} {
		if err := fn(); err != first {
			t.Fatalf("first failure replaced: %v / %v", first, err)
		}
	}
}

func TestRequestConcurrentSharedCap(t *testing.T) {
	b := New(nil, Limits{MaxRequestWork: 1000})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if err := b.ChargeMany(map[Counter]int64{CounterReadOps: 1}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if b.Used(CounterRequestWork) != 1000 || b.Used(CounterReadOps) != 1000 {
		t.Fatalf("shared counts: %v", b.Snapshot())
	}
	if err := b.Charge(CounterReadOps, 1); err == nil {
		t.Fatal("aggregate cap reset")
	}
}

func TestRequestZeroUnlimitedAndOverflow(t *testing.T) {
	b := New(nil, Limits{})
	if err := b.ChargeMany(map[Counter]int64{CounterInputBytes: math.MaxInt64, CounterReadOps: 1}); err == nil {
		t.Fatal("aggregate overflow accepted")
	}
	if len(b.Snapshot()) != 0 {
		t.Fatal("overflow mutated counters")
	}
	if _, err := CheckedProduct(math.MaxInt64, 2); err == nil {
		t.Fatal("allocation overflow accepted")
	}
	if _, err := CheckedProduct(-1, 2); err == nil {
		t.Fatal("negative allocation accepted")
	}
	if v, err := CheckedProduct(math.MaxInt64, 0); err != nil || v != 0 {
		t.Fatalf("zero product: %d %v", v, err)
	}
	b = New(nil, Limits{})
	if err := b.Charge(CounterInputBytes, 1<<30); err != nil {
		t.Fatalf("zero ceased to mean unlimited: %v", err)
	}
}
