package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Abstract post-simulation graphs isolate binding ownership and refusal. They
// do not claim verifier validity. The original-first disjoint-lifetime and
// per-iteration JVM fixtures exercise actual decoding, definitions and output.
func TestDynamicCaptureWebKeepsOtherLifetimesAndEachOriginalProducer(t *testing.T) {
	for _, variant := range []string{"original", "two snapshots", "no conflict", "entry", "missing definition", "mixed domain", "narrow boolean", "narrow byte", "narrow char", "narrow short", "different physical head", "wrong load kind", "wrong store kind", "shared IINC", "missing LOAD seed", "result snapshot", "wrong snapshot PC", "deep forwarding", "parameter", "custom", "stack alias", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			shared := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			op := func(pc uint16, code int) *OpCode {
				return &OpCode{CurrentOffset: pc, Instr: &Instruction{OpCode: code}}
			}
			prior, first, second, load, site := op(0, OP_ISTORE_3), op(2, OP_ISTORE_3), op(4, OP_ISTORE_3), op(6, OP_ILOAD_3), op(7, OP_INVOKEDYNAMIC)
			producer1, producer2 := values.NewJavaLiteral(17, typ), values.NewJavaLiteral(-29, typ)
			prior.stackConsumed = []values.JavaValue{values.NewJavaLiteral(42, typ)}
			first.stackConsumed = []values.JavaValue{producer1}
			second.stackConsumed = []values.JavaValue{producer2}
			produced := values.NewSlotValue(shared, typ)
			load.stackProduced = []values.JavaValue{produced}
			// Stack Pop and recorded stackProduced use separate forwarding
			// wrappers, while their logical source ref is initially shared.
			seed := values.NewSlotValue(shared, typ)
			d := NewDecompiler(nil, nil)
			d.opCodes = []*OpCode{prior, first, second, load, site}
			d.cachedSlotWebs = &slotWeb{webOf: map[*OpCode]int{prior: 1, first: 2, second: 2, load: 2}, entryWeb: map[int]int{3: 0}}
			d.opcodeIdToRef = map[*OpCode][][2]any{prior: {{shared, true}}, first: {{shared, true}}, second: {{shared, false}}}
			d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{site: {{Value: seed, Operand: true, OriginPC: 7, ExpectedType: typ}}}
			want := variant == "original" || variant == "two snapshots"
			switch variant {
			case "two snapshots":
				copyLoad := op(7, OP_ILOAD_3)
				site.CurrentOffset = 8
				d.evaluationSnapshots[site][0].OriginPC = 8
				copyLoad.stackProduced = []values.JavaValue{values.NewSlotValue(shared, typ)}
				d.opCodes = []*OpCode{prior, first, second, load, copyLoad, site}
				d.cachedSlotWebs.webOf[copyLoad] = 2
				d.evaluationSnapshots[site] = append(d.evaluationSnapshots[site], EvaluationSnapshot{Value: values.NewSlotValue(shared, typ), Operand: true, OriginPC: 8, ExpectedType: typ})
			case "no conflict":
				d.opcodeIdToRef[prior] = [][2]any{{values.NewJavaRef(utils.NewRootVariableId(), nil, typ), true}}
			case "entry":
				d.cachedSlotWebs.entryWeb[3] = 2
			case "missing definition":
				delete(d.opcodeIdToRef, second)
			case "mixed domain":
				second.stackConsumed = []values.JavaValue{values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
			case "narrow boolean", "narrow byte", "narrow char", "narrow short":
				name := map[string]string{"narrow boolean": types.JavaBoolean, "narrow byte": types.JavaByte, "narrow char": types.JavaChar, "narrow short": types.JavaShort}[variant]
				d.evaluationSnapshots[site][0].ExpectedType = types.NewJavaPrimer(name)
			case "different physical head":
				second.Instr.OpCode = OP_ISTORE_2
			case "wrong load kind":
				load.Instr.OpCode = OP_ALOAD_3
			case "wrong store kind":
				second.Instr.OpCode = OP_ASTORE_3
			case "shared IINC":
				increment := op(5, OP_IINC)
				increment.Data = []byte{3, 1}
				increment.Source = []*OpCode{second}
				d.opCodes = []*OpCode{prior, first, second, increment, load, site}
			case "missing LOAD seed":
				load.stackProduced = []values.JavaValue{values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, typ), typ)}
			case "result snapshot":
				d.evaluationSnapshots[site][0].Operand = false
			case "wrong snapshot PC":
				d.evaluationSnapshots[site][0].OriginPC++
			case "deep forwarding":
				var forwarded values.JavaValue = shared
				for i := 0; i < 33; i++ {
					forwarded = values.NewSlotValue(forwarded, typ)
				}
				d.evaluationSnapshots[site][0].Value = forwarded
			case "parameter":
				shared.IsParam = true
			case "custom":
				shared.CustomValue = &values.CustomValue{}
			case "stack alias":
				shared.StackVar = producer1
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.partitionSharedDynamicCaptureWebs()
			bound := d.opcodeIdToRef[first][0][0].(*values.JavaRef)
			if (bound != shared) != want {
				t.Fatalf("web rebound=%v want=%v", bound != shared, want)
			}
			if d.opcodeIdToRef[prior][0][0] == bound && want {
				t.Fatal("other lifetime was rebound")
			}
			if want {
				if d.opcodeIdToRef[second][0][0] != bound || values.UnpackSoltValue(seed) != bound || values.UnpackSoltValue(produced) != bound || first.stackConsumed[0] != producer1 || second.stackConsumed[0] != producer2 {
					t.Fatal("definition/LOAD/snapshot identity or original producer changed")
				}
				for _, snapshot := range d.evaluationSnapshots[site] {
					if values.UnpackSoltValue(snapshot.Value) != bound {
						t.Fatal("another operand snapshot kept the stale lifetime")
					}
				}
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("refusal did not retain current budget error")
			}
		})
	}
}
