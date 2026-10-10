package javaclassparser

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestRequestCanceledBeforeParseEvenWithSharedBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	work := workbudget.New(nil, Limits{})
	r, err := DecompileWithOptions([]byte{0}, DecompileOptions{Context: ctx, Work: work})
	if !errors.Is(err, context.Canceled) || r.Status != "canceled" || r.Source != "" || r.InputHash != "" {
		t.Fatalf("parse ran before cancel admission: %+v %v", r, err)
	}
	if len(work.Snapshot()) != 0 {
		t.Fatalf("canceled request performed work: %v", work.Snapshot())
	}
}

func TestRequestInputCapPrecedesMagicAndHash(t *testing.T) {
	work := workbudget.New(nil, Limits{MaxInputBytes: 3})
	r, err := DecompileWithOptions([]byte{0, 0, 0, 0}, DecompileOptions{Work: work})
	var be *workbudget.Error
	if !errors.As(err, &be) || be.Counter != workbudget.CounterInputBytes || r.Status != "resource_limit" || r.InputHash != "" {
		t.Fatalf("input gate too late: %+v %v", r, err)
	}
	if len(work.Snapshot()) != 0 {
		t.Fatal("rejected input was charged")
	}
}

func TestRequestParserAllocationRejectedBeforeConstantPool(t *testing.T) {
	raw := make([]byte, 110)
	binary.BigEndian.PutUint32(raw, 0xcafebabe)
	binary.BigEndian.PutUint16(raw[6:], 52)
	binary.BigEndian.PutUint16(raw[8:], 100)
	work := workbudget.New(nil, Limits{MaxParseItems: 10})
	obj, err := ParseWithBudget(raw, work)
	var be *workbudget.Error
	if obj != nil || !errors.As(err, &be) || be.Counter != workbudget.CounterParseItems {
		t.Fatalf("constant pool allocated/read before limit: %v %v", obj, err)
	}
	if work.Used(workbudget.CounterReadBytes) != 10 || work.Used(workbudget.CounterParseItems) != 0 {
		t.Fatalf("unexpected preallocation work: %v", work.Snapshot())
	}
}

func TestRequestReaderSubregionsAndTransactions(t *testing.T) {
	work := workbudget.New(nil, Limits{MaxReadBytes: 2})
	root, err := NewClassReaderWithBudget([]byte{1, 2, 3, 4}, work)
	if err != nil {
		t.Fatal(err)
	}
	child := root.Subreader(2)
	if child.readUint16() != 258 {
		t.Fatal("legal child read failed")
	}
	snap := work.Snapshot()
	if child.readUint8() != 0 || child.Err() == nil || root.Err() == nil {
		t.Fatal("attribute borrowed parent bytes")
	}
	if child.Offset() != 2 || root.Offset() != 2 || !reflect.DeepEqual(snap, work.Snapshot()) {
		t.Fatal("failed child advanced or charged")
	}
	work = workbudget.New(nil, Limits{MaxReadBytes: 1})
	r, _ := NewClassReaderWithBudget([]byte{1, 2}, work)
	snap = work.Snapshot()
	if r.readUint16() != 0 || !workbudget.Is(r.Err()) || r.Offset() != 0 || !reflect.DeepEqual(snap, work.Snapshot()) {
		t.Fatal("failed read partially committed")
	}
}

type cancelDuringParse struct {
	context.Context
	calls atomic.Int32
}

func (c *cancelDuringParse) Err() error {
	if c.calls.Add(1) > 20 {
		return context.Canceled
	}
	return nil
}

func TestRequestCancellationDuringParserAndSharedResolver(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelDuringParse{Context: context.Background()}
	work := workbudget.New(nil, Limits{})
	r, err := DecompileWithOptions(raw, DecompileOptions{Context: ctx, Work: work})
	if !errors.Is(err, context.Canceled) || r.Status != "canceled" || r.Source != "" || work.Used(workbudget.CounterReadBytes) == 0 || work.Used(workbudget.CounterReadBytes) >= int64(len(raw)) {
		t.Fatalf("parser did not cancel midway: %+v %v %v", r, err, work.Snapshot())
	}
	work = workbudget.New(nil, Limits{MaxInputBytes: int64(len(raw))*2 - 1})
	if _, err := ParseWithBudget(raw, work); err != nil {
		t.Fatal(err)
	}
	dumper := NewClassObjectDumper(NewClassObject())
	dumper.Work = work
	if _, err := dumper.parseResolved(raw); !workbudget.Is(err) {
		t.Fatalf("resolver parse reset request budget: %v", err)
	}
	if work.Used(workbudget.CounterInputBytes) != int64(len(raw)) {
		t.Fatal("failed second input partially charged")
	}
}

func TestRequestReaderBudgetLeavesStandaloneParseUnchanged(t *testing.T) {
	raw, err := classes.FS.ReadFile("IfTest.class")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	work := workbudget.New(nil, Limits{})
	budgeted, err := ParseWithBudget(raw, work)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain.Bytes(), budgeted.Bytes()) {
		t.Fatal("budgeted reader changed parsed class")
	}
	if work.Used(workbudget.CounterInputBytes) != int64(len(raw)) || work.Used(workbudget.CounterReadBytes) == 0 || work.Used(workbudget.CounterParseItems) == 0 {
		t.Fatalf("parser work missing: %v", work.Snapshot())
	}
}
