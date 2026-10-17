package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func localPostTestProtocol(typ string, subtract bool, slot int, wide bool) (*Decompiler, []*OpCode, *values.JavaRef, values.JavaValue) {
	p := localPostProtocols[typ]
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(typ))
	old := values.NewSlotValue(ref, ref.Type())
	op := p.add
	if subtract {
		op = p.sub
	}
	codes := []int{p.load, p.dup, p.one, op}
	if p.narrow != 0 {
		codes = append(codes, p.narrow)
	}
	codes = append(codes, p.store)
	var path []*OpCode
	offset := 10
	for i, code := range codes {
		n := &OpCode{Instr: &Instruction{OpCode: code}, CurrentOffset: uint16(offset)}
		width := 1
		if i == 0 || i == len(codes)-1 {
			n.Data, width = []byte{byte(slot)}, 2
			if wide {
				n.IsWide, n.Data, width = true, []byte{byte(slot >> 8), byte(slot)}, 4
			} else if slot < 4 {
				base := OP_ILOAD_0 + (code-OP_ILOAD)*4
				if i > 0 {
					base = OP_ISTORE_0 + (code-OP_ISTORE)*4
				}
				n.Instr.OpCode, n.Data, width = base+slot, nil, 1
			}
		}
		if i > 0 {
			prev := path[i-1]
			prev.Target, n.Source = []*OpCode{n}, []*OpCode{prev}
		}
		path = append(path, n)
		offset += width
	}
	path[0].stackProduced = []values.JavaValue{old}
	d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{old: path[0]}}
	return d, path, ref, old
}

func TestLocalPostUpdateKeepsTypedProtocolAndBinding(t *testing.T) {
	for _, typ := range []string{types.JavaInteger, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		for _, subtract := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/subtract=%t", typ, subtract), func(t *testing.T) {
				for _, slot := range []int{0, 1, 3, 4, 255, 256, 4095} {
					for _, wide := range []bool{false, true} {
						if !wide && slot > 255 {
							continue
						}
						d, path, ref, old := localPostTestProtocol(typ, subtract, slot, wide)
						want := values.INC
						if subtract {
							want = values.DEC
						}
						store := path[len(path)-1]
						for repeat := 0; repeat < 2; repeat++ {
							store.LocalStoreFoldedToPostOp = repeat > 0
							if got, ok := d.immediateLocalPostUpdate(store, ref, old); !ok || got != want {
								t.Fatalf("slot=%d wide=%t repeat=%d got=%s allowed=%t", slot, wide, repeat, got, ok)
							}
						}
					}
				}
			})
		}
	}
}

func TestLocalPostUpdateRejectsAmbiguousOrChangedEvidence(t *testing.T) {
	for _, change := range []string{"nil decompiler", "nil store", "nil ref", "nil old", "nil instruction", "wrong load", "wrong store", "other slot", "wrong dup", "wrong constant", "wrong arithmetic", "missing narrowing", "wrong narrowing", "same spelling", "copied ref", "copied word", "changed binding", "custom ref", "stack ref", "receiver", "boolean", "producer absent", "other producer", "produced absent", "malformed load", "malformed store", "wide fixed load", "wide fixed store", "budget", "canceled"} {
		t.Run(change, func(t *testing.T) {
			d, path, ref, old := localPostTestProtocol(types.JavaByte, false, 1, false)
			store := path[len(path)-1]
			switch change {
			case "nil decompiler":
				d = nil
			case "nil store":
				store = nil
			case "nil ref":
				ref = nil
			case "nil old":
				old = nil
			case "nil instruction":
				path[2].Instr = nil
			case "wrong load":
				path[0].Instr.OpCode = OP_ALOAD_1
			case "wrong store":
				store.Instr.OpCode = OP_ASTORE_1
			case "other slot":
				store.Instr.OpCode = OP_ISTORE_2
			case "wrong dup":
				path[1].Instr.OpCode = OP_DUP2
			case "wrong constant":
				path[2].Instr.OpCode = OP_ICONST_2
			case "wrong arithmetic":
				path[3].Instr.OpCode = OP_IMUL
			case "missing narrowing":
				path[4].Instr.OpCode = OP_NOP
			case "wrong narrowing":
				path[4].Instr.OpCode = OP_I2C
			case "same spelling":
				ref = values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
			case "copied ref":
				copy := *ref
				ref = &copy
			case "copied word":
				old = values.NewSlotValue(ref, ref.Type())
			case "changed binding":
				old.(*values.SlotValue).ResetValue(values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type()))
			case "custom ref":
				ref.CustomValue = &values.CustomValue{}
			case "stack ref":
				ref.StackVar = values.NewJavaLiteral(0, ref.Type())
			case "receiver":
				ref.IsThis = true
			case "boolean":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "producer absent":
				d.stackValueProducers = nil
			case "other producer":
				d.stackValueProducers[old] = path[1]
			case "produced absent":
				path[0].stackProduced = nil
			case "malformed load":
				path[0].Data = []byte{1}
			case "malformed store":
				store.Data = []byte{1}
			case "wide fixed load":
				path[0].IsWide = true
			case "wide fixed store":
				store.IsWide = true
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if _, ok := d.immediateLocalPostUpdate(store, ref, old); ok {
				t.Fatal("invalid protocol admitted")
			}
		})
	}
	for _, change := range []string{"custom", "catch", "try parent", "extra entry", "missing entry", "extra target", "wrong target", "gap", "back edge", "data", "wide", "handler boundary"} {
		for position := 0; position < 6; position++ {
			if (change == "extra entry" || change == "missing entry" || change == "gap" || change == "back edge") && position == 0 || (change == "extra target" || change == "wrong target") && position == 5 || (change == "data" || change == "wide") && (position == 0 || position == 5) {
				continue
			}
			t.Run(fmt.Sprintf("%s/position=%d", change, position), func(t *testing.T) {
				d, path, ref, old := localPostTestProtocol(types.JavaByte, false, 1, false)
				n := path[position]
				switch change {
				case "custom":
					n.IsCustom = true
				case "catch":
					n.IsCatch = true
				case "try parent":
					n.IsTryCatchParent = true
				case "extra entry":
					n.Source = append(n.Source, &OpCode{})
				case "missing entry":
					n.Source = nil
				case "extra target":
					n.Target = append(n.Target, &OpCode{})
				case "wrong target":
					n.Target = []*OpCode{{}}
				case "gap":
					n.CurrentOffset++
				case "back edge":
					n.CurrentOffset = 0
				case "data":
					n.Data = []byte{0}
				case "wide":
					n.IsWide = true
				case "handler boundary":
					d.ExceptionTable = []*ExceptionTableEntry{{StartPc: n.CurrentOffset, EndPc: n.CurrentOffset + 1, HandlerPc: 100}}
				}
				if _, ok := d.immediateLocalPostUpdate(path[5], ref, old); ok {
					t.Fatal("changed original edge admitted")
				}
			})
		}
	}
}

func TestLocalPostUpdateReplacesOnlySurvivingWord(t *testing.T) {
	d, path, ref, old := localPostTestProtocol(types.JavaLong, false, 1, false)
	sim := NewStackSimulation(NewEmptyStackEntry(), map[int]*values.JavaRef{1: ref}, utils.NewRootVariableId())
	sim.Push(old)
	sim.Push(old)
	shared := sim.stackEntry
	post := values.NewBinaryExpression(ref, values.NewJavaLiteral(int64(1), ref.Type()), values.INC, ref.Type())
	proxy := NewStackSimulationProxy(sim, func(values.JavaValue) { t.Fatal("invented decoded push") }, func() values.JavaValue { t.Fatal("invented decoded pop"); return nil })
	proxy.ReplaceTop(post)
	if sim.Size() != 2 || sim.Peek() != post || shared.value != old || shared.parent.value != old || old.(*values.SlotValue).GetValue() != ref || len(path[0].stackProduced) != 1 || d.stackValueProducers[old] != path[0] {
		t.Fatal("shared old value or physical producer mutated")
	}
	if sim.Pop() != post || sim.Pop() != old {
		t.Fatal("earlier operand rewritten")
	}
}

func TestLocalPostUpdateReloadNeedsItsOwnOriginalBinding(t *testing.T) {
	for _, change := range []string{"valid", "wide", "same handlers", "wrong slot", "wrong category", "malformed read", "missing produced word", "same word", "same spelling", "copied ref", "wrong producer", "stale binding"} {
		t.Run(change, func(t *testing.T) {
			d, path, ref, old := localPostTestProtocol(types.JavaChar, false, 1, false)
			reload := path[1]
			reload.Instr.OpCode = OP_ILOAD_1
			word := values.NewSlotValue(ref, ref.Type())
			reload.stackProduced = []values.JavaValue{word}
			d.stackValueProducers[word] = reload
			want := false
			switch change {
			case "valid":
				want = true
			case "wide":
				path[0].Instr.OpCode, path[0].IsWide, path[0].Data = OP_ILOAD, true, []byte{1, 44}
				reload.Instr.OpCode, reload.IsWide, reload.Data, reload.CurrentOffset = OP_ILOAD, true, []byte{1, 44}, 14
				for i := 2; i < 6; i++ {
					path[i].CurrentOffset += 6
				}
				path[5].Instr.OpCode, path[5].IsWide, path[5].Data = OP_ISTORE, true, []byte{1, 44}
				want = true
			case "same handlers":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 30, HandlerPc: 40}}
				want = true
			case "wrong slot":
				reload.Instr.OpCode = OP_ILOAD_2
			case "wrong category":
				reload.Instr.OpCode = OP_ALOAD_1
			case "malformed read":
				reload.Data = []byte{1}
			case "missing produced word":
				reload.stackProduced = nil
			case "same word":
				reload.stackProduced = []values.JavaValue{old}
			case "same spelling":
				word.ResetValue(values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type()))
			case "copied ref":
				copy := *ref
				word.ResetValue(&copy)
			case "wrong producer":
				d.stackValueProducers[word] = path[0]
			case "stale binding":
				reload.stackProduced = append(reload.stackProduced, values.NewSlotValue(ref, ref.Type()))
			}
			if _, ok := d.immediateLocalPostUpdate(path[5], ref, old); ok != want {
				t.Fatalf("permission=%t want=%t", ok, want)
			}
		})
	}
}
