package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNumericConditionalDiscoveryWithoutConditionalStoreSkipsWebs(t *testing.T) {
	store := op(OP_ISTORE_1, 0)
	store.stackConsumed = []values.JavaValue{values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))}
	d := &Decompiler{
		opCodes:        make([]*OpCode, 1024),
		opcodeIdToRef:  map[*OpCode][][2]any{store: nil},
		cachedSlotWebs: &slotWeb{webOf: map[*OpCode]int{store: 0}},
		Work:           workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1, MaxOutputBytes: 1}),
	}
	d.restoreNumericConditionalWebs()
	if d.Work.Err() != nil || d.Work.Used(workbudget.CounterGraphScans) != 1 || d.Work.Used(workbudget.CounterIntermediateBytes) != 0 {
		t.Fatal("nonconditional store constructed source webs", d.Work.Err())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d = &Decompiler{Work: workbudget.New(ctx, workbudget.Limits{})}
	d.restoreNumericConditionalWebs()
	if d.Work.Err() == nil {
		t.Fatal("empty inventory concealed cancellation")
	}
}

func TestNumericJoinDiscoveryWithoutMultipleDefinitionsSkipsWebs(t *testing.T) {
	for _, opcode := range []int{OP_DSTORE_1, OP_ASTORE_1} {
		store := op(opcode, 0)
		var typ types.JavaType = types.NewJavaPrimer(types.JavaDouble)
		var rhs values.JavaValue = values.NewJavaLiteral(7.0, typ)
		if opcode == OP_ASTORE_1 {
			typ = types.NewJavaClass("java.lang.Object")
			rhs = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		}
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		store.stackConsumed = []values.JavaValue{rhs}
		d := &Decompiler{
			opcodeIdToRef:  map[*OpCode][][2]any{store: {{ref, true}}},
			cachedSlotWebs: &slotWeb{webOf: map[*OpCode]int{store: 0}, entryWeb: map[int]int{0: 1}},
			Work:           workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1, MaxOutputBytes: 1}),
		}
		d.unifyNumericExitWebs()
		if d.Work.Err() != nil || d.Work.Used(workbudget.CounterGraphScans) != 1 || d.Work.Used(workbudget.CounterIntermediateBytes) != 0 {
			t.Fatal("single store constructed numeric join webs", d.Work.Err())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &Decompiler{Work: workbudget.New(ctx, workbudget.Limits{})}
	d.unifyNumericExitWebs()
	if d.Work.Err() == nil {
		t.Fatal("empty join inventory concealed cancellation")
	}
}

func TestNumericDiscoveryDisabledPolicySkipsAllInventoryWork(t *testing.T) {
	for _, pass := range []string{"conditional", "join"} {
		t.Run(pass, func(t *testing.T) {
			d := &Decompiler{
				Env: func(name string) string {
					if name == "JDEC_LIVEINTERVAL_OFF" {
						return "1"
					}
					return ""
				},
				opcodeIdToRef: map[*OpCode][][2]any{op(OP_ISTORE_1, 0): nil, op(OP_ISTORE_2, 1): nil},
				Work:          workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1, MaxOutputBytes: 1}),
			}
			if pass == "conditional" {
				d.restoreNumericConditionalWebs()
			} else {
				d.unifyNumericExitWebs()
			}
			if d.Work.Err() != nil || d.Work.Used(workbudget.CounterGraphScans) != 0 || d.Work.Used(workbudget.CounterIntermediateBytes) != 0 {
				t.Fatal("disabled pass consumed inventory work", d.Work.Err())
			}
		})
	}
}

func TestNumericDiscoveryChargesInventoryBeforeCandidateSelection(t *testing.T) {
	for _, pass := range []string{"conditional", "join"} {
		t.Run(pass, func(t *testing.T) {
			d := &Decompiler{opcodeIdToRef: map[*OpCode][][2]any{}, Work: workbudget.New(nil, workbudget.Limits{MaxGraphScans: 4})}
			typ := types.NewJavaPrimer(types.JavaInteger)
			for i := 0; i < 4; i++ {
				store := op(OP_ISTORE_1, uint16(i))
				var value values.JavaValue = values.NewJavaLiteral(7, typ)
				if pass == "conditional" {
					value = values.NewTernaryExpression(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), value, values.NewJavaLiteral(-1, typ))
				}
				store.stackConsumed = []values.JavaValue{value}
				d.opcodeIdToRef[store] = [][2]any{{nil, true}}
			}
			candidate := d.hasMultipleNumericStores
			if pass == "conditional" {
				candidate = d.hasNumericConditionalStore
			}
			if !candidate() || d.Work.Err() != nil || d.Work.Used(workbudget.CounterGraphScans) != 4 {
				t.Fatal("inventory cost depends on which candidate map iteration visits first", d.Work.Snapshot(), d.Work.Err())
			}
		})
	}
}
