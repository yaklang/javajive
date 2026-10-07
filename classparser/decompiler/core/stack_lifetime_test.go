package core

import (
	"context"
	"errors"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Enumerate finite graphs from an independent adjacency matrix. The oracle
// enumerates routes from every graph entry to the write and checks producer
// membership; it does not walk the production predecessor closure. This certifies
// placement, not arbitrary JVM equivalence (the original-first banks do that
// for their tested values, stores, exceptions and effects).
func TestBoundedStackLifetimeProducerPlacementModel(t *testing.T) {
	pairs := [][2]int{{0, 1}, {0, 2}, {0, 3}, {1, 2}, {1, 3}, {2, 3}}
	cases := 0
	for a := 0; a < 4; a++ {
		for b := 0; b < 4; b++ {
			if a == b {
				continue
			}
			for c := 0; c < 4; c++ {
				if c == a || c == b {
					continue
				}
				dpc := 6 - a - b - c
				pcs := []int{a, b, c, dpc}
				for mask := 0; mask < 64; mask++ {
					adj := [4][4]bool{}
					in := [4]int{}
					nodes := make([]*OpCode, 4)
					for i := range nodes {
						nodes[i] = &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: uint16(pcs[i] + 10)}
					}
					for bit, p := range pairs {
						if mask&(1<<bit) != 0 {
							adj[p[0]][p[1]] = true
							in[p[1]]++
							nodes[p[0]].Target = append(nodes[p[0]].Target, nodes[p[1]])
							nodes[p[1]].Source = append(nodes[p[1]].Source, nodes[p[0]])
						}
					}
					for source := 0; source < 4; source++ {
						for target := 0; target < 4; target++ {
							var routes [][]int
							var visit func(int, []int)
							visit = func(n int, path []int) {
								path = append(append([]int(nil), path...), n)
								if n == target {
									routes = append(routes, path)
									return
								}
								for next := 0; next < 4; next++ {
									if adj[n][next] {
										visit(next, path)
									}
								}
							}
							for entry := 0; entry < 4; entry++ {
								if in[entry] == 0 {
									visit(entry, nil)
								}
							}
							want := source == target || len(routes) > 0
							if source != target {
								for _, route := range routes {
									found := false
									for _, n := range route {
										if n == source {
											found = true
										}
										if found && pcs[n] < pcs[source] {
											want = false
										}
									}
									want = want && found
								}
							}
							decoder := &Decompiler{}
							if got := decoder.stackLifetimeProducerPath(nodes[source], nodes[target]); got != want {
								t.Fatalf("mask%d pcs%v %d->%d: %v want%v routes%v", mask, pcs, source, target, got, want, routes)
							}
							cases++
						}
					}
				}
			}
		}
	}
	if cases != 24576 {
		t.Fatal("incomplete finite graph domain", cases)
	}
	decoder := &Decompiler{}
	if decoder.stackLifetimeProducerPath(nil, nil) {
		t.Fatal("missing original producer accepted")
	}
	for length := 510; length <= 513; length++ {
		first := &OpCode{Instr: InstrInfos[OP_FLOAD_0], CurrentOffset: 1}
		last := first
		for i := 0; i < length; i++ {
			next := &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: uint16(i + 2), Source: []*OpCode{last}}
			last.Target = []*OpCode{next}
			last = next
		}
		if decoder.stackLifetimeProducerPath(first, last) != (length < 512) {
			t.Fatal("bounded path boundary", length)
		}
	}
	t.Logf("independent finite producer placement graphs=%d", cases)
}

func TestStackLifetimeCopiesKeepOriginalTimeValueIdentityAndSharedStack(t *testing.T) {
	count := 0
	for _, typ := range []string{types.JavaBoolean, types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaFloat, types.JavaDouble, types.JavaLong} {
		for _, kind := range []int{OP_ISTORE_0, OP_FSTORE_0, OP_DSTORE_0, OP_LSTORE_0, OP_ASTORE_0, OP_IINC, OP_PUTSTATIC, OP_PUTFIELD, OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE} {
			for _, producerKind := range []int{OP_ILOAD_0, OP_INVOKESTATIC, OP_FALOAD} {
				for copies := 1; copies <= 4; copies++ {
					producer := &OpCode{Instr: InstrInfos[producerKind], CurrentOffset: 10}
					rhs := &OpCode{Instr: InstrInfos[OP_INVOKESTATIC], CurrentOffset: 20, Source: []*OpCode{producer}}
					write := &OpCode{Instr: InstrInfos[kind], CurrentOffset: 30, Source: []*OpCode{rhs}}
					producer.Target = []*OpCode{rhs}
					rhs.Target = []*OpCode{write}
					value := values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaPrimer(typ) })
					sim := NewStackSimulation(NewEmptyStackEntry(), map[int]*values.JavaRef{}, utils.NewRootVariableId())
					for i := 0; i < copies; i++ {
						sim.Push(value)
					}
					original := sim.stackEntry
					d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{value: producer}, ExceptionTable: []*ExceptionTableEntry{{StartPc: 10, EndPc: 15, HandlerPc: 50}, {StartPc: 20, EndPc: 35, HandlerPc: 60}}}
					if err := d.preserveStackAcrossWrite(sim, write); err != nil {
						t.Fatal(err)
					}
					captures := d.evaluationSnapshots[producer]
					if len(captures) != 1 || len(d.evaluationSnapshots) != 1 || captures[0].Value != value || captures[0].OriginPC != 10 || !captures[0].Operand || len(d.disFoldRef) != 1 || sim.Size() != copies {
						t.Fatal("original time/value or exactly once capture changed")
					}
					for s := sim.stackEntry; s.parent != nil; s = s.parent {
						if s.value != captures[0].Ref || s.value.Type().String(nil) != typ {
							t.Fatal("duplicated stack values lost identity/type")
						}
					}
					for s := original; s.parent != nil; s = s.parent {
						if s.value != value {
							t.Fatal("shared predecessor stack mutated")
						}
					}
					if err := d.preserveStackAcrossWrite(sim, write); err != nil || len(d.evaluationSnapshots[producer]) != 1 {
						t.Fatal("saved value recaptured", err)
					}
					count++
				}
			}
		}
	}
	if count != 1536 {
		t.Fatal("incomplete primitive/store/copy model", count)
	}
	t.Logf("typed stack lifetime/handler/copy controls=%d", count)
}

func TestStackLifetimePlanRefusesMissingPathAndCancellationAtomically(t *testing.T) {
	for _, fault := range []string{"alternate entry", "missing producer instruction", "back edge", "second invalid operand", "canceled", "budget"} {
		producer := &OpCode{Instr: InstrInfos[OP_FLOAD_0], CurrentOffset: 10}
		write := &OpCode{Instr: InstrInfos[OP_FSTORE_0], CurrentOffset: 20, Source: []*OpCode{producer}}
		producer.Target = []*OpCode{write}
		value := values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaPrimer(types.JavaFloat) })
		sim := NewStackSimulation(NewEmptyStackEntry(), map[int]*values.JavaRef{}, utils.NewRootVariableId())
		sim.Push(value)
		old := sim.stackEntry
		d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{value: producer}}
		switch fault {
		case "second invalid operand":
			other := values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaPrimer(types.JavaFloat) })
			d.stackValueProducers[other] = &OpCode{Instr: InstrInfos[OP_FLOAD_1], CurrentOffset: 5}
			sim.Push(other)
			old = sim.stackEntry
		case "alternate entry":
			write.Source = append(write.Source, &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: 15})
		case "missing producer instruction":
			producer.Instr = nil
		case "back edge":
			write.CurrentOffset = 5
		case "canceled":
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			d.Work = workbudget.New(ctx, workbudget.Limits{})
		case "budget":
			middle := &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: 15, Source: []*OpCode{producer}, Target: []*OpCode{write}}
			producer.Target = []*OpCode{middle}
			write.Source = []*OpCode{middle}
			d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
		}
		err := d.preserveStackAcrossWrite(sim, write)
		if fault == "budget" || fault == "canceled" {
			var classified *workbudget.Error
			if !errors.As(err, &classified) {
				t.Fatal("lost structured budget/cancel classification", fault, err)
			}
		}
		// An unregistered/unknown producer is outside this proof domain; it cannot
		// authorize a copy. Malformed known paths and exhausted budgets refuse.
		if fault != "missing producer instruction" && err == nil {
			t.Fatal("invalid lifetime proof accepted", fault)
		}
		if len(d.evaluationSnapshots) != 0 || len(d.stackLifetimeCopies) != 0 || len(d.disFoldRef) != 0 {
			t.Fatal("partial plan published", fault)
		}
		if err != nil && sim.stackEntry != old {
			t.Fatal("failed plan mutated shared stack", fault)
		}
	}
}

func TestStackLifetimeUseViewsKeepEarlierConsumersAndEvidenceSeparate(t *testing.T) {
	for _, reject := range []bool{false, true} {
		typ := types.NewJavaPrimer(types.JavaFloat)
		local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		original := values.NewSlotValue(local, typ)
		producer := &OpCode{Instr: InstrInfos[OP_FLOAD_0], CurrentOffset: 10}
		write := &OpCode{Instr: InstrInfos[OP_FSTORE_0], CurrentOffset: 20, Source: []*OpCode{producer}}
		producer.Target = []*OpCode{write}
		d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{original: producer}}
		early, err := d.stackLifetimeUseView(original)
		if err != nil || early == original {
			t.Fatal("consumer lacks private view", err)
		}
		sim := NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())
		sim.Push(original)
		predecessor := sim.stackEntry
		if reject {
			write.Source = append(write.Source, &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: 15})
		}
		err = d.preserveStackAcrossWrite(sim, write)
		if reject {
			if err == nil || early.(*values.SlotValue).GetValue() != original || len(d.evaluationSnapshots) != 0 {
				t.Fatal("failed plan changed an earlier use")
			}
		} else {
			saved := d.stackLifetimeCopies[original]
			late, e := d.stackLifetimeUseView(original)
			if e != nil || saved == nil || early.(*values.SlotValue).GetValue() != saved || late != saved || d.evaluationSnapshots[producer][0].Value != original {
				t.Fatal("earlier/later consumer or snapshot RHS changed", e)
			}
		}
		if predecessor.value != original || original.GetValue() != local {
			t.Fatal("immutable predecessor/source evidence changed")
		}
	}
}

func TestStackLifetimePureReadsCommuteOnlyWithUnrelatedWrites(t *testing.T) {
	for _, writeKind := range []int{OP_FSTORE_0, OP_FSTORE_1, OP_PUTSTATIC, OP_FASTORE} {
		for _, observable := range []bool{false, true} {
			typ := types.NewJavaPrimer(types.JavaFloat)
			local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			var value values.JavaValue = values.NewSlotValue(local, typ)
			if observable {
				value = values.TagEffects(value, values.EffectThrow)
			}
			producer := &OpCode{Instr: InstrInfos[OP_FLOAD_0], CurrentOffset: 10}
			write := &OpCode{Instr: InstrInfos[writeKind], CurrentOffset: 20, Source: []*OpCode{producer}}
			producer.Target = []*OpCode{write}
			d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{value: producer}}
			sim := NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())
			sim.Push(value)
			if err := d.preserveStackAcrossWrite(sim, write); err != nil {
				t.Fatal(err)
			}
			want := observable || writeKind == OP_FSTORE_0
			if got := len(d.evaluationSnapshots) != 0; got != want {
				t.Fatalf("write%d observable%v copied%v want%v", writeKind, observable, got, want)
			}
		}
	}
	typ := types.NewJavaPrimer(types.JavaInteger)
	value := values.TagEffects(values.NewJavaLiteral(3, typ), values.EffectWriteMemory)
	write := &OpCode{Instr: InstrInfos[OP_IINC], CurrentOffset: 10}
	d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{value: write}}
	sim := NewStackSimulation(NewEmptyStackEntry(), nil, utils.NewRootVariableId())
	sim.Push(value)
	if err := d.preserveStackAcrossWrite(sim, write); err != nil || len(d.evaluationSnapshots) != 0 {
		t.Fatal("writer's own output recaptured before its consumer", err)
	}
}

func TestStackLifetimeDominanceRetainsLoopsAndRejectsUnknownEntries(t *testing.T) {
	for _, kind := range []string{"closed loop", "alternate entry", "unreachable producer", "duplicate predecessor", "duplicate target", "edge bound"} {
		p := &OpCode{Instr: InstrInfos[OP_FLOAD_0], CurrentOffset: 1}
		a := &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: 2}
		b := &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: 3}
		w := &OpCode{Instr: InstrInfos[OP_FSTORE_0], CurrentOffset: 4}
		p.Target = []*OpCode{a}
		a.Source = []*OpCode{p, b}
		a.Target = []*OpCode{b, w}
		b.Source = []*OpCode{a}
		b.Target = []*OpCode{a}
		w.Source = []*OpCode{a}
		switch kind {
		case "alternate entry":
			a.Source = append(a.Source, &OpCode{Instr: InstrInfos[OP_NOP], CurrentOffset: 2, Target: []*OpCode{a}})
		case "unreachable producer":
			a.Source = []*OpCode{b}
		case "duplicate predecessor":
			a.Source = append(a.Source, p)
		case "duplicate target":
			p.Target = append(p.Target, a)
		case "edge bound":
			for i := 0; i < 8193; i++ {
				p.Target = append(p.Target, b)
			}
		}
		if got := (&Decompiler{}).stackLifetimeProducerPath(p, w); got != (kind == "closed loop") {
			t.Fatalf("%s: %v", kind, got)
		}
	}
}

func TestStackLifetimeConstructorWitnessNeedsExactOriginalProducer(t *testing.T) {
	for _, fault := range []string{"valid", "different value", "different origin", "missing operand", "duplicate snapshot", "missing produced word", "unregistered producer", "different copy", "parameter", "this"} {
		typ := types.NewJavaPrimer(types.JavaFloat)
		value := values.NewCustomValue(nil, func() types.JavaType { return typ })
		p := &OpCode{Instr: InstrInfos[OP_INVOKESTATIC], CurrentOffset: 10, stackProduced: []values.JavaValue{value}}
		ref := values.NewJavaRef(utils.NewRootVariableId(), value, typ)
		d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{value: p}, stackLifetimeCopies: map[values.JavaValue]*values.JavaRef{value: ref}, evaluationSnapshots: map[*OpCode][]EvaluationSnapshot{p: {{Ref: ref, Value: value, OriginPC: 10, Operand: true}}}}
		switch fault {
		case "different value":
			d.evaluationSnapshots[p][0].Value = values.NewJavaLiteral(3, typ)
		case "different origin":
			d.evaluationSnapshots[p][0].OriginPC++
		case "missing operand":
			d.evaluationSnapshots[p][0].Operand = false
		case "duplicate snapshot":
			d.evaluationSnapshots[p] = append(d.evaluationSnapshots[p], d.evaluationSnapshots[p][0])
		case "missing produced word":
			p.stackProduced = nil
		case "unregistered producer":
			d.stackValueProducers = nil
		case "different copy":
			d.stackLifetimeCopies[value] = values.NewJavaRef(utils.NewRootVariableId(), value, typ)
		case "parameter":
			ref.IsParam = true
		case "this":
			ref.IsThis = true
		}
		if got := d.stackLifetimeSnapshotProducer(value, ref, p); got != (fault == "valid") {
			t.Fatalf("%s: %v", fault, got)
		}
	}
}

func TestStackLifetimeUseViewBudgetDoesNotPublishOrReturnNilOperand(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaFloat)
	original := values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, typ), typ)
	p := &OpCode{Instr: InstrInfos[OP_FLOAD_0], CurrentOffset: 1}
	d := &Decompiler{stackValueProducers: map[values.JavaValue]*OpCode{original: p}, Work: workbudget.New(nil, workbudget.Limits{MaxNodeCopies: 1})}
	if _, err := d.stackLifetimeUseView(original); err != nil {
		t.Fatal(err)
	}
	prior := d.stackLifetimeUseViews[original][0]
	got, err := d.stackLifetimeUseView(original)
	var classified *workbudget.Error
	if got != original || !errors.As(err, &classified) || len(d.stackLifetimeUseViews[original]) != 1 || prior.GetValue() != original {
		t.Fatal("failed view allocation changed IR/diagnostic or produced nil operand", err)
	}
}
