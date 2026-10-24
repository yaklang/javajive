package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestLocalPreUpdateRequiresPrivateOriginalEdgeAndBinding(t *testing.T) {
	for _, change := range []string{"valid", "negative", "wide", "wide negative", "explicit load", "same handlers", "repeat proof", "nil decompiler", "nil update", "nil load", "nil instruction", "wrong update", "wrong load", "extra target", "extra entry", "wrong predecessor", "gap", "back edge", "other slot", "same spelling", "copied reference", "missing reference", "receiver", "narrow type", "long type", "custom update", "custom load", "catch update", "catch load", "try parent", "postfix already consumed", "handler boundary", "wide wrong slot", "malformed update", "malformed load", "nonunit delta", "zero delta", "budget", "canceled"} {
		t.Run(change, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			loaded := values.JavaValue(values.NewSlotValue(ref, ref.Type()))
			update := &OpCode{Instr: &Instruction{OpCode: OP_IINC}, CurrentOffset: 10, Data: []byte{1, 1}, Ref: ref}
			load := &OpCode{Instr: &Instruction{OpCode: OP_ILOAD_1}, CurrentOffset: 13, Source: []*OpCode{update}}
			update.Target = []*OpCode{load}
			d := &Decompiler{}
			want, delta := false, 1
			switch change {
			case "valid":
				want = true
			case "negative":
				update.Data[1], want, delta = 255, true, -1
			case "wide", "wide negative", "wide wrong slot":
				update.IsWide, update.Data, load.CurrentOffset = true, []byte{1, 44, 0, 1}, 16
				load.Instr.OpCode, load.IsWide, load.Data = OP_ILOAD, true, []byte{1, 44}
				want = change != "wide wrong slot"
				if change == "wide negative" {
					update.Data[2], update.Data[3], delta = 255, 255, -1
				}
				if change == "wide wrong slot" {
					load.Data[1]++
				}
			case "explicit load":
				load.Instr.OpCode, load.Data, want = OP_ILOAD, []byte{1}, true
			case "same handlers":
				d.ExceptionTable, want = []*ExceptionTableEntry{{StartPc: 0, EndPc: 20, HandlerPc: 30}}, true
			case "repeat proof":
				update.IincFoldedToPreOp, want = true, true
			case "nil decompiler":
				d = nil
			case "nil update":
				update = nil
			case "nil load":
				load = nil
			case "nil instruction":
				load.Instr = nil
			case "wrong update":
				update.Instr.OpCode = OP_NOP
			case "wrong load":
				load.Instr.OpCode = OP_ALOAD_1
			case "extra target":
				update.Target = append(update.Target, load)
			case "extra entry":
				load.Source = append(load.Source, &OpCode{})
			case "wrong predecessor":
				load.Source[0] = &OpCode{}
			case "gap":
				load.CurrentOffset++
			case "back edge":
				load.CurrentOffset = 9
			case "other slot":
				load.Instr.OpCode = OP_ILOAD_2
			case "same spelling":
				loaded = values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
			case "copied reference":
				copy := *ref
				loaded = &copy
			case "missing reference":
				update.Ref = nil
			case "receiver":
				ref.IsThis = true
			case "narrow type":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaByte))
			case "long type":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "custom update":
				update.IsCustom = true
			case "custom load":
				load.IsCustom = true
			case "catch update":
				update.IsCatch = true
			case "catch load":
				load.IsCatch = true
			case "try parent":
				load.IsTryCatchParent = true
			case "postfix already consumed":
				update.IincFoldedToPostOp = true
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 13, HandlerPc: 30}}
			case "malformed update":
				update.Data = []byte{1}
			case "malformed load":
				load.Instr.OpCode = OP_ILOAD
				load.Data = []byte{1, 2}
			case "nonunit delta":
				update.Data[1], want, delta = 2, true, 2
			case "zero delta":
				update.Data[1], want, delta = 0, true, 0
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				_ = d.Work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			gotDelta, got := d.immediateLocalPreUpdate(update, load, loaded)
			if got != want || got && gotDelta != delta {
				t.Fatalf("permission=%t delta=%d want=%t delta=%d", got, gotDelta, want, delta)
			}
		})
	}
}

// Exercise every byte operand and the signed-wide boundaries independently of
// source rewriting. Neither operand width nor zero addition changes ownership.
func TestLocalPreUpdateRetainsSignedByteAndWideDeltas(t *testing.T) {
	for _, wide := range []bool{false, true} {
		deltas := []int{}
		slots := []int{1, 255}
		if wide {
			deltas = []int{-32768, -32767, -129, -128, -1, 0, 1, 2, 127, 128, 255, 256, 32766, 32767}
			slots = []int{1, 255, 256, 4095}
		} else {
			for n := -128; n <= 127; n++ {
				deltas = append(deltas, n)
			}
		}
		t.Run(fmt.Sprintf("wide=%t", wide), func(t *testing.T) {
			for _, slot := range slots {
				for _, delta := range deltas {
					ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
					update := &OpCode{Instr: &Instruction{OpCode: OP_IINC}, CurrentOffset: 10, Data: []byte{byte(slot), byte(delta)}, Ref: ref}
					load := &OpCode{Instr: &Instruction{OpCode: OP_ILOAD}, CurrentOffset: 13, Data: []byte{byte(slot)}, Source: []*OpCode{update}}
					if wide {
						update.IsWide = true
						update.Data = make([]byte, 4)
						binary.BigEndian.PutUint16(update.Data, uint16(slot))
						binary.BigEndian.PutUint16(update.Data[2:], uint16(int16(delta)))
						load.CurrentOffset = 16
						load.IsWide = true
						load.Data = make([]byte, 2)
						binary.BigEndian.PutUint16(load.Data, uint16(slot))
					}
					update.Target = []*OpCode{load}
					if got, ok := (&Decompiler{}).immediateLocalPreUpdate(update, load, values.NewSlotValue(ref, ref.Type())); !ok || got != delta {
						t.Fatalf("slot=%d delta=%d got=%d allowed=%t", slot, delta, got, ok)
					}
				}
			}
		})
	}
}
