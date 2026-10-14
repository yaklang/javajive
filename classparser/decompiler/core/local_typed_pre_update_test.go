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

func localTypedPreTestProtocol(typ string, subtract bool, slot int, wide bool) (*Decompiler, []*OpCode, *values.JavaRef, *values.JavaRef, values.JavaValue, values.JavaValue) {
	d, path, ref, loaded := localPostTestProtocol(typ, subtract, slot, wide)
	p := localPostProtocols[typ]
	n := len(path)
	path[1].Instr.OpCode = p.one
	path[2].Instr.OpCode = p.add
	if subtract {
		path[2].Instr.OpCode = p.sub
	}
	if p.narrow != 0 {
		path[3].Instr.OpCode = p.narrow
	}
	dup := path[n-2]
	dup.Instr.OpCode = p.dup
	op := values.ADD
	if subtract {
		op = values.SUB
	}
	word := values.JavaValue(values.NewBinaryExpression(loaded, values.NewJavaLiteral(1, ref.Type()), op, ref.Type()))
	if p.narrow != 0 {
		word = &values.CastExpression{Value: word, TargetType: ref.Type().Copy()}
	}
	producer := path[n-3]
	producer.stackProduced = []values.JavaValue{word}
	d.stackValueProducers[word] = producer
	tmp := values.NewJavaRef(utils.NewRootVariableId().Next(), word, word.Type().Copy())
	tmp.MarkOriginalStackMaterialization(int(dup.CurrentOffset), p.dup, word)
	remaining := values.NewSlotValue(tmp, tmp.Type())
	stored := values.NewSlotValue(tmp, tmp.Type())
	dup.stackProduced = []values.JavaValue{remaining}
	d.stackValueProducers[remaining] = dup
	d.dupConvertedRefValue = map[*OpCode][]values.JavaValue{dup: {word}}
	d.opcodeIdToRef = map[*OpCode][][2]any{dup: {{tmp, true}}}
	return d, path, ref, tmp, remaining, stored
}

func TestLocalTypedPreUpdateKeepsNewWordAndMaterialization(t *testing.T) {
	for _, typ := range []string{types.JavaInteger, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		for _, subtract := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/subtract=%t", typ, subtract), func(t *testing.T) {
				for _, slot := range []int{0, 1, 3, 4, 255, 256, 4095} {
					for _, wide := range []bool{false, true} {
						if !wide && slot > 255 {
							continue
						}
						d, path, ref, _, remaining, stored := localTypedPreTestProtocol(typ, subtract, slot, wide)
						want := values.INC
						if subtract {
							want = values.DEC
						}
						for repeat := 0; repeat < 2; repeat++ {
							path[len(path)-1].LocalStoreFoldedToTypedPreOp = repeat > 0
							path[len(path)-2].LocalUpdateDupFolded = repeat > 0
							dup, target, op, ok := d.immediateLocalTypedPreUpdate(path[len(path)-1], ref, remaining, stored)
							if !ok || op != want || dup != path[len(path)-2] || target != path[0].stackProduced[0] {
								t.Fatalf("slot=%d wide=%t repeat=%d op=%s permission=%t", slot, wide, repeat, op, ok)
							}
						}
					}
				}
			})
		}
	}
}

func TestLocalTypedPreUpdateRequiresOriginalNumericProducers(t *testing.T) {
	for _, change := range []string{"valid", "same handlers", "nil decompiler", "nil store", "nil ref", "nil surviving", "nil stored", "receiver", "custom target", "stack target", "boolean target", "other slot", "wrong load", "wrong constant", "wrong arithmetic", "wrong conversion", "wrong dup", "stored other temp", "surviving copy", "materialization absent", "temporary receiver", "temporary parameter", "temporary custom", "temporary stack", "temporary type", "word type", "producer absent", "wrong producer", "load absent", "copied load binding", "remaining absent", "wrong remaining producer", "converted absent", "converted other", "extra converted", "infos absent", "other info ref", "not fresh", "extra info", "custom dup", "alternate entry", "forked producer", "gap", "wide constant", "handler boundary", "budget", "canceled"} {
		t.Run(change, func(t *testing.T) {
			d, path, ref, tmp, remaining, stored := localTypedPreTestProtocol(types.JavaByte, false, 1, false)
			store, producer, dup := path[5], path[3], path[4]
			word := producer.stackProduced[0]
			want := false
			switch change {
			case "valid":
				want = true
			case "same handlers":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 30, HandlerPc: 40}}
				want = true
			case "nil decompiler":
				d = nil
			case "nil store":
				store = nil
			case "nil ref":
				ref = nil
			case "nil surviving":
				remaining = nil
			case "nil stored":
				stored = nil
			case "receiver":
				ref.IsThis = true
			case "custom target":
				ref.CustomValue = &values.CustomValue{}
			case "stack target":
				ref.StackVar = word
			case "boolean target":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "other slot":
				store.Instr.OpCode = OP_ISTORE_2
			case "wrong load":
				path[0].Instr.OpCode = OP_ALOAD_1
			case "wrong constant":
				path[1].Instr.OpCode = OP_ICONST_2
			case "wrong arithmetic":
				path[2].Instr.OpCode = OP_IMUL
			case "wrong conversion":
				producer.Instr.OpCode = OP_I2C
			case "wrong dup":
				dup.Instr.OpCode = OP_DUP2
			case "stored other temp":
				stored = values.NewJavaRef(utils.NewRootVariableId(), word, ref.Type())
			case "surviving copy":
				remaining = values.NewSlotValue(tmp, tmp.Type())
			case "materialization absent":
				tmp = values.NewJavaRef(utils.NewRootVariableId(), word, ref.Type())
				remaining.(*values.SlotValue).ResetValue(tmp)
				stored.(*values.SlotValue).ResetValue(tmp)
				d.opcodeIdToRef[dup][0][0] = tmp
			case "temporary receiver":
				tmp.IsThis = true
			case "temporary parameter":
				tmp.IsParam = true
			case "temporary custom":
				tmp.CustomValue = &values.CustomValue{}
			case "temporary stack":
				tmp.StackVar = word
			case "temporary type":
				tmp.ResetVarType(types.NewJavaPrimer(types.JavaChar))
			case "word type":
				word.(*values.CastExpression).TargetType = types.NewJavaPrimer(types.JavaChar)
			case "producer absent":
				producer.stackProduced = nil
			case "wrong producer":
				d.stackValueProducers[word] = path[2]
			case "load absent":
				path[0].stackProduced = nil
			case "copied load binding":
				copy := *ref
				path[0].stackProduced[0].(*values.SlotValue).ResetValue(&copy)
			case "remaining absent":
				dup.stackProduced = nil
			case "wrong remaining producer":
				d.stackValueProducers[remaining] = path[0]
			case "converted absent":
				d.dupConvertedRefValue[dup] = nil
			case "converted other":
				d.dupConvertedRefValue[dup][0] = values.NewJavaLiteral(1, ref.Type())
			case "extra converted":
				d.dupConvertedRefValue[dup] = append(d.dupConvertedRefValue[dup], word)
			case "infos absent":
				d.opcodeIdToRef[dup] = nil
			case "other info ref":
				copy := *tmp
				d.opcodeIdToRef[dup][0][0] = &copy
			case "not fresh":
				d.opcodeIdToRef[dup][0][1] = false
			case "extra info":
				d.opcodeIdToRef[dup] = append(d.opcodeIdToRef[dup], d.opcodeIdToRef[dup][0])
			case "custom dup":
				dup.IsCustom = true
			case "alternate entry":
				dup.Source = append(dup.Source, &OpCode{})
			case "forked producer":
				producer.Target = append(producer.Target, &OpCode{})
			case "gap":
				dup.CurrentOffset++
			case "wide constant":
				path[1].IsWide = true
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: dup.CurrentOffset, EndPc: store.CurrentOffset, HandlerPc: 40}}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, _, _, ok := d.immediateLocalTypedPreUpdate(store, ref, remaining, stored)
			if ok != want {
				t.Fatalf("permission=%t want=%t", ok, want)
			}
		})
	}
}

func localTypedReloadTestProtocol(typ string, subtract bool, slot int, wide bool) (*Decompiler, []*OpCode, *values.JavaRef, values.JavaValue) {
	d, path, ref, old := localPostTestProtocol(typ, subtract, slot, wide)
	p := localPostProtocols[typ]
	path[1].Instr.OpCode = p.one
	path[2].Instr.OpCode = p.add
	if subtract {
		path[2].Instr.OpCode = p.sub
	}
	if p.narrow != 0 {
		path[3].Instr.OpCode = p.narrow
	}
	store, reload := path[len(path)-2], path[len(path)-1]
	store.Instr.OpCode, store.Data, store.IsWide = reload.Instr.OpCode, reload.Data, reload.IsWide
	reload.Instr.OpCode, reload.Data, reload.IsWide = path[0].Instr.OpCode, path[0].Data, path[0].IsWide
	offset := 10
	for _, node := range path {
		node.CurrentOffset = uint16(offset)
		width, _ := localPostInstructionWidth(node)
		offset += width
	}
	word := values.JavaValue(values.NewBinaryExpression(old, values.NewJavaLiteral(1, ref.Type()), values.ADD, ref.Type()))
	if subtract {
		word = values.NewBinaryExpression(old, values.NewJavaLiteral(1, ref.Type()), values.SUB, ref.Type())
	}
	if p.narrow != 0 {
		word = &values.CastExpression{Value: word, TargetType: ref.Type().Copy()}
	}
	producer := path[len(path)-3]
	producer.stackProduced = []values.JavaValue{word}
	d.stackValueProducers[word] = producer
	d.slotStoreValue = map[*OpCode]values.JavaValue{store: word}
	d.opcodeIdToRef = map[*OpCode][][2]any{store: {{ref, false}}}
	return d, path, ref, values.NewSlotValue(ref, ref.Type())
}

func TestLocalTypedReloadUpdateRequiresSameOriginalBindingAndProducer(t *testing.T) {
	for _, typ := range []string{types.JavaInteger, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		for _, subtract := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/subtract=%t", typ, subtract), func(t *testing.T) {
				for _, slot := range []int{0, 1, 3, 4, 255, 256, 4095} {
					for _, wide := range []bool{false, true} {
						if !wide && slot > 255 {
							continue
						}
						d, path, _, loaded := localTypedReloadTestProtocol(typ, subtract, slot, wide)
						want := values.INC
						if subtract {
							want = values.DEC
						}
						for repeat := 0; repeat < 2; repeat++ {
							path[len(path)-2].LocalStoreFoldedToTypedPreOp = repeat > 0
							op, ok := d.immediateLocalTypedReloadUpdate(path[len(path)-1], loaded)
							if !ok || op != want {
								t.Fatalf("slot=%d wide=%t repeat=%d op=%s ok=%t", slot, wide, repeat, op, ok)
							}
						}
					}
				}
			})
		}
	}
	for _, change := range []string{"valid", "original consumer view", "post result already owns write", "ordinary wrapper", "redirected view", "nil decompiler", "nil reload", "nil loaded", "receiver", "custom ref", "stack ref", "boolean", "different store ref", "fresh store", "store info absent", "wrong store word", "producer absent", "wrong producer", "copied old ref", "wrong load slot", "wrong store slot", "wrong reload slot", "wrong reload opcode", "wrong one", "wrong arithmetic", "wrong narrowing", "alternate entry", "fork", "gap", "malformed reload", "handler boundary", "budget", "cancel"} {
		t.Run(change, func(t *testing.T) {
			d, path, ref, loaded := localTypedReloadTestProtocol(types.JavaByte, false, 1, false)
			reload, store, producer := path[5], path[4], path[3]
			want := false
			switch change {
			case "valid":
				want = true
			case "nil decompiler":
				d = nil
			case "post result already owns write":
				store.LocalStoreFoldedToPostOp = true
			case "original consumer view":
				d.slotStoreValue[store] = values.NewStackLifetimeUseView(producer.stackProduced[0])
				want = true
			case "ordinary wrapper":
				d.slotStoreValue[store] = values.NewSlotValue(producer.stackProduced[0], ref.Type())
			case "redirected view":
				view := values.NewStackLifetimeUseView(producer.stackProduced[0])
				view.ResetValue(values.NewJavaLiteral(1, ref.Type()))
				d.slotStoreValue[store] = view
			case "nil reload":
				reload = nil
			case "nil loaded":
				loaded = nil
			case "receiver":
				ref.IsThis = true
			case "custom ref":
				ref.CustomValue = &values.CustomValue{}
			case "stack ref":
				ref.StackVar = producer.stackProduced[0]
			case "boolean":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "different store ref":
				copy := *ref
				d.opcodeIdToRef[store][0][0] = &copy
			case "fresh store":
				d.opcodeIdToRef[store][0][1] = true
			case "store info absent":
				delete(d.opcodeIdToRef, store)
			case "wrong store word":
				d.slotStoreValue[store] = values.NewJavaLiteral(1, ref.Type())
			case "producer absent":
				producer.stackProduced = nil
			case "wrong producer":
				d.stackValueProducers[producer.stackProduced[0]] = path[2]
			case "copied old ref":
				copy := *ref
				path[0].stackProduced[0].(*values.SlotValue).ResetValue(&copy)
			case "wrong load slot":
				path[0].Instr.OpCode = OP_ILOAD_2
			case "wrong store slot":
				store.Instr.OpCode = OP_ISTORE_2
			case "wrong reload slot":
				reload.Instr.OpCode = OP_ILOAD_2
			case "wrong reload opcode":
				reload.Instr.OpCode = OP_ALOAD_1
			case "wrong one":
				path[1].Instr.OpCode = OP_ICONST_2
			case "wrong arithmetic":
				path[2].Instr.OpCode = OP_IMUL
			case "wrong narrowing":
				producer.Instr.OpCode = OP_I2C
			case "alternate entry":
				store.Source = append(store.Source, &OpCode{})
			case "fork":
				path[1].Target = append(path[1].Target, &OpCode{})
			case "gap":
				store.CurrentOffset++
			case "malformed reload":
				reload.IsWide = true
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: reload.CurrentOffset, EndPc: reload.CurrentOffset + 1, HandlerPc: 40}}
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, ok := d.immediateLocalTypedReloadUpdate(reload, loaded)
			if ok != want {
				t.Fatalf("permission=%t want=%t", ok, want)
			}
		})
	}
}
