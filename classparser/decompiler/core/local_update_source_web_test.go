package core

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceUpdateWebFixture(shape, kind string, subtract bool) (*Decompiler, *OpCode, *OpCode, *OpCode, values.JavaValue) {
	var d *Decompiler
	var path []*OpCode
	var ref *values.JavaRef
	var target values.JavaValue
	var producer, store *OpCode
	switch shape {
	case "post":
		d, path, ref, target = localPostTestProtocol(kind, subtract, 1, false)
		store = path[len(path)-1]
		producer = store
		store.LocalStoreFoldedToPostOp = true
		d.opcodeIdToRef = map[*OpCode][][2]any{store: {{ref, false}}}
	case "pre duplicate":
		var remaining, stored values.JavaValue
		d, path, ref, _, remaining, stored = localTypedPreTestProtocol(kind, subtract, 1, false)
		store = path[len(path)-1]
		producer = store
		store.LocalStoreFoldedToTypedPreOp = true
		path[len(path)-2].LocalUpdateDupFolded = true
		d.slotStoreValue = map[*OpCode]values.JavaValue{store: stored}
		d.opcodeIdToRef[store] = [][2]any{{ref, false}}
		_, target, _, _ = d.immediateLocalTypedPreUpdate(store, ref, remaining, stored)
	case "pre reload":
		d, path, ref, target = localTypedReloadTestProtocol(kind, subtract, 1, false)
		producer = path[len(path)-1]
		store = path[len(path)-2]
		store.LocalStoreFoldedToTypedPreOp = true
	default:
		panic(shape)
	}
	entry := op(OP_START, 0)
	path[0].Source = []*OpCode{entry}
	entry.Target = []*OpCode{path[0]}
	d.opCodes = append([]*OpCode{entry}, path...)
	operation := values.INC
	if subtract {
		operation = values.DEC
	}
	var expr *values.JavaExpression
	if shape == "post" {
		expr = values.NewBinaryExpression(target, values.NewJavaLiteral(1, ref.Type()), operation, ref.Type())
	} else {
		expr = values.NewUnaryExpression(target, operation, ref.Type())
	}
	d.stackValueProducers[expr] = producer
	// Later disjoint reuse of the same physical slot must stay independent.
	separate := op(store.Instr.OpCode, 100)
	separate.Source = []*OpCode{producer}
	producer.Target = []*OpCode{separate}
	if len(store.Data) > 0 {
		separate.Data = append([]byte{}, store.Data...)
	}
	d.opCodes = append(d.opCodes, separate)
	return d, path[0], store, separate, expr
}

func TestLocalUpdateSourcePartitionCouplesMutationAndPreservesOriginalWeb(t *testing.T) {
	for _, shape := range []string{"post", "pre duplicate", "pre reload"} {
		for _, kind := range []string{types.JavaInteger, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaLong, types.JavaFloat, types.JavaDouble} {
			for _, subtract := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/subtract=%t", shape, kind, subtract), func(t *testing.T) {
					d, load, store, separate, _ := sourceUpdateWebFixture(shape, kind, subtract)
					original := d.slotWebs()
					before := map[*OpCode]int{}
					for op, w := range original.webOf {
						before[op] = w
					}
					if original.webOf[load] == original.webOf[store] {
						t.Fatal("fixture must have distinct input/output reaching definitions")
					}
					for repeat := 0; repeat < 2; repeat++ {
						source := d.localUpdateSourceWebs()
						if source == nil || source.webOf[load] != source.webOf[store] || source.webOf[load] == source.webOf[separate] {
							t.Fatal("missing update constraint or unrelated slot reuse merged")
						}
						if source.entryWeb[1] != source.webOf[load] {
							t.Fatal("entry input lost its source location")
						}
						if d.cachedSlotWebs != original || !reflect.DeepEqual(original.webOf, before) {
							t.Fatal("source projection changed original reaching partition")
						}
					}
				})
			}
		}
	}
}

func TestLocalUpdateSourcePartitionRejectsBorrowedPacketsAndBoundsWork(t *testing.T) {
	for _, change := range []string{"valid", "flag only", "different target", "fresh declaration", "extra store info", "wrong operator", "wrong literal", "opaque literal", "private fork", "gap", "handler boundary", "nil successor", "no produced constraint", "budget", "canceled"} {
		t.Run(change, func(t *testing.T) {
			d, load, store, _, value := sourceUpdateWebFixture("post", types.JavaLong, false)
			expr := value.(*values.JavaExpression)
			want := false
			switch change {
			case "valid":
				want = true
			case "flag only":
				delete(d.stackValueProducers, expr.Values[0])
			case "different target":
				copy := *values.UnpackSoltValue(expr.Values[0]).(*values.JavaRef)
				d.opcodeIdToRef[store][0][0] = &copy
			case "fresh declaration":
				d.opcodeIdToRef[store][0][1] = true
			case "extra store info":
				d.opcodeIdToRef[store] = append(d.opcodeIdToRef[store], d.opcodeIdToRef[store][0])
			case "wrong operator":
				expr.Op = values.DEC
			case "wrong literal":
				expr.Values[1] = values.NewJavaLiteral(2, expr.Type())
			case "opaque literal":
				expr.Values[1] = values.NewJavaLiteral([]byte{1}, expr.Type())
			case "private fork":
				load.Target = append(load.Target, &OpCode{})
			case "gap":
				store.CurrentOffset++
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: store.CurrentOffset, EndPc: store.CurrentOffset + 1, HandlerPc: 130}}
			case "nil successor":
				store.LocalStoreFoldedToPostOp = false
				store.LocalStoreFoldedToTypedPreOp = false
				store.Source = []*OpCode{nil}
				expr.Values = expr.Values[:1]
			case "no produced constraint":
				delete(d.stackValueProducers, expr)
			case "budget":
				d.slotWebs()
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				d.slotWebs()
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			source := d.localUpdateSourceWebs()
			joined := source != nil && source.webOf[load] == source.webOf[store]
			if joined != want {
				t.Fatalf("coupled=%t want=%t", joined, want)
			}
		})
	}
}
