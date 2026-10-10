package core

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func assertSameDefs(t *testing.T, label string, a, b []*OpCode, ea, eb bool) {
	t.Helper()
	if ea != eb || !reflect.DeepEqual(defOffsets(a), defOffsets(b)) {
		t.Fatalf("%s: sparse %v entry=%v vs other %v entry=%v", label, defOffsets(a), ea, defOffsets(b), eb)
	}
}

func TestT25C01FixedPointMatchesOracleAndProduction(t *testing.T) {
	// loop + iinc + parameter (existing production fixture)
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_IINC, 0, 1, OP_ILOAD_0, OP_IFNE, 255, 252, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	inc := d.opCodes[d.offsetToOpcodeIndex[2]]
	prod, pEntry := g.ReachingDefinitions(inc, 0)
	oracle, oEntry, _ := IndependentReachingOracle(g, inc, 0)
	sparse := NewSparseReaching(g)
	got, gEntry, err := sparse.Definitions(inc, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertSameDefs(t, "T25-C01 loop/iinc production", got, prod, gEntry, pEntry)
	assertSameDefs(t, "T25-C01 loop/iinc oracle", got, oracle, gEntry, oEntry)

	// handler throw-site locals (not try-exit)
	d2 := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_ICONST_1, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_ICONST_2, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_RETURN, OP_ASTORE_1, OP_ILOAD_0, OP_IRETURN})
	d2.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 12, HandlerPc: 13, CatchType: 1}}
	g2, err := d2.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	load := d2.opCodes[d2.offsetToOpcodeIndex[14]]
	prod2, e2 := g2.ReachingDefinitions(load, 0)
	oracle2, oe2, _ := IndependentReachingOracle(g2, load, 0)
	s2 := NewSparseReaching(g2)
	got2, ge2, err := s2.Definitions(load, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertSameDefs(t, "T25-C01 handler production", got2, prod2, ge2, e2)
	assertSameDefs(t, "T25-C01 handler oracle", got2, oracle2, ge2, oe2)
	if e2 || !reflect.DeepEqual(defOffsets(got2), []uint16{3, 8}) {
		t.Fatalf("T25-C01 handler defs %v entry=%v", defOffsets(got2), ge2)
	}
}

func TestT25C02Category2Overlap(t *testing.T) {
	d := auditCFG(t, []byte{OP_LCONST_0, OP_LSTORE_0, OP_ICONST_1, OP_ISTORE_1, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	ret := g.Nodes[len(g.Nodes)-1]
	sparse := NewSparseReaching(g)
	defs0, inv0, err := sparse.Definitions(ret, 0)
	if err != nil {
		t.Fatal(err)
	}
	prod0, pInv0 := g.ReachingDefinitions(ret, 0)
	assertSameDefs(t, "T25-C02 slot0", defs0, prod0, inv0, pInv0)
	if len(defs0) != 0 || !inv0 {
		t.Fatalf("T25-C02 cat2 head must be invalidated, defs=%v invalid=%v", defOffsets(defs0), inv0)
	}
	defs1, inv1, err := sparse.Definitions(ret, 1)
	if err != nil {
		t.Fatal(err)
	}
	prod1, pInv1 := g.ReachingDefinitions(ret, 1)
	assertSameDefs(t, "T25-C02 slot1", defs1, prod1, inv1, pInv1)
	if len(defs1) != 1 || inv1 {
		t.Fatalf("T25-C02 cat1 store at slot1 must survive as its own def, got %v entry=%v", defOffsets(defs1), inv1)
	}
}

func TestT25C03SparseSlotWork(t *testing.T) {
	for _, extra := range []int{32, 64, 128} {
		code := []byte{OP_ICONST_0, OP_ISTORE_0}
		for i := 0; i < extra; i++ {
			code = append(code, OP_ILOAD_0, OP_ICONST_1, OP_IADD, OP_POP)
		}
		code = append(code, OP_ILOAD_0, OP_IRETURN)
		d := auditCFG(t, code)
		g, err := d.buildSemanticCFG()
		if err != nil {
			t.Fatal(err)
		}
		load := g.Nodes[len(g.Nodes)-2]
		_, _, slowCopies := IndependentReachingOracle(g, load, 0)
		sparse := NewSparseReaching(g)
		got, entry, err := sparse.Definitions(load, 0)
		if err != nil {
			t.Fatal(err)
		}
		prod, pEntry := g.ReachingDefinitions(load, 0)
		assertSameDefs(t, "T25-C03 facts", got, prod, entry, pEntry)
		if slowCopies == 0 {
			t.Fatal("T25-C03 oracle performed no copy work")
		}
		if sparse.Merges() > slowCopies {
			t.Fatalf("T25-C03 N=%d sparse merges %d exceeded slow copies %d", extra, sparse.Merges(), slowCopies)
		}
		t.Logf("T25-C03 N=%d slow_copies=%d sparse_merges=%d visits=%d", extra, slowCopies, sparse.Merges(), sparse.Visits())
	}
}

func TestT25C03RepeatMeasurements(t *testing.T) {
	code := []byte{OP_ICONST_0, OP_ISTORE_0}
	for i := 0; i < 64; i++ {
		code = append(code, OP_ILOAD_0, OP_ICONST_1, OP_IADD, OP_POP)
	}
	code = append(code, OP_ILOAD_0, OP_IRETURN)
	d := auditCFG(t, code)
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	load := g.Nodes[len(g.Nodes)-2]
	for i := 0; i < 10; i++ {
		_, _, slowCopies := IndependentReachingOracle(g, load, 0)
		s := NewSparseReaching(g)
		got, entry, err := s.Definitions(load, 0)
		if err != nil {
			t.Fatal(err)
		}
		prod, pEntry := g.ReachingDefinitions(load, 0)
		assertSameDefs(t, "repeat facts", got, prod, entry, pEntry)
		t.Logf("repeat=%d slow_copies=%d sparse_merges=%d visits=%d", i, slowCopies, s.Merges(), s.Visits())
	}
}

func TestT25C04DenseJoinQueryCache(t *testing.T) {
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_IINC, 0, 1, OP_ILOAD_0, OP_IFNE, 255, 252, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	inc := d.opCodes[d.offsetToOpcodeIndex[2]]
	s := NewSparseReaching(g)
	first, e1, err := s.Definitions(inc, 0)
	if err != nil {
		t.Fatal(err)
	}
	merges := s.Merges()
	visits := s.Visits()
	for i := 0; i < 50; i++ {
		again, e2, err := s.Definitions(inc, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertSameDefs(t, "T25-C04 cached", again, first, e2, e1)
	}
	if s.Merges() != merges || s.Visits() != visits {
		t.Fatalf("T25-C04 repeated queries recomputed: merges %d→%d visits %d→%d", merges, s.Merges(), visits, s.Visits())
	}
}

func TestT25C05CacheAliasIsolation(t *testing.T) {
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_ILOAD_0, OP_IRETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	load := g.Nodes[2]
	s := NewSparseReaching(g)
	defs, entry, err := s.Definitions(load, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) == 0 {
		t.Fatal("T25-C05 expected a store definition")
	}
	defs[0] = nil
	again, entry2, err := s.Definitions(load, 0)
	if err != nil {
		t.Fatal(err)
	}
	if entry != entry2 || len(again) == 0 || again[0] == nil {
		t.Fatalf("T25-C05 caller mutation poisoned cache: %v entry=%v", again, entry2)
	}
}

func TestT25C06BudgetCountsInsertsNotPops(t *testing.T) {
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_IINC, 0, 1, OP_ILOAD_0, OP_IFNE, 255, 252, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	inc := d.opCodes[d.offsetToOpcodeIndex[2]]
	g.Work = workbudget.New(nil, workbudget.Limits{MaxSetElementWork: 1})
	s := NewSparseReaching(g)
	_, _, err = s.Definitions(inc, 0)
	if err == nil {
		t.Fatal("T25-C06 set-insert budget must fire; queue pops alone must not sneak past the limit")
	}
	if !workbudget.Is(err) {
		t.Fatalf("T25-C06 want workbudget.Error, got %v", err)
	}
}

func attachStaticLongParam(d *Decompiler) {
	d.FunctionContext.IsStatic = true
	d.FunctionType = types.NewJavaFuncType("(J)I", []types.JavaType{types.NewJavaPrimer(types.JavaLong)}, types.NewJavaPrimer(types.JavaInteger))
}

func TestT25EntryWideParamTailWrite(t *testing.T) {
	if independentCategoryWidth(OP_LSTORE_0) != 2 || independentCategoryWidth(OP_ISTORE_1) != 1 {
		t.Fatal("independent opcode table must treat lstore as cat2 and istore as cat1")
	}
	d := auditCFG(t, []byte{OP_ICONST_1, OP_ISTORE_1, OP_LLOAD_0, OP_L2I, OP_IRETURN})
	attachStaticLongParam(d)
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	if g.liveInWidth(0) != 2 {
		t.Fatalf("static (J) live-in slot 0 must be cat2, width=%d", g.liveInWidth(0))
	}
	if !g.liveInIsCat2Tail(1) {
		t.Fatal("slot 1 must be the tail of the live-in long")
	}
	load := d.opCodes[d.offsetToOpcodeIndex[2]]
	prod, pEntry := g.ReachingDefinitions(load, 0)
	oracle, oEntry, _ := IndependentReachingOracle(g, load, 0)
	lin, lEntry := LinearCat2Occupancy(g, load, 0)
	assertSameDefs(t, "T25 entry-long vs oracle", prod, oracle, pEntry, oEntry)
	assertSameDefs(t, "T25 entry-long vs linear cat2 interpreter", prod, lin, pEntry, lEntry)
	if len(prod) != 0 || !pEntry {
		t.Fatalf("write to tail of live-in long must invalidate slot 0, defs=%v entry=%v", defOffsets(prod), pEntry)
	}
	prod1, e1 := g.ReachingDefinitions(load, 1)
	oracle1, oe1, _ := IndependentReachingOracle(g, load, 1)
	lin1, le1 := LinearCat2Occupancy(g, load, 1)
	assertSameDefs(t, "T25 entry-long tail vs oracle", prod1, oracle1, e1, oe1)
	assertSameDefs(t, "T25 entry-long tail vs linear", prod1, lin1, e1, le1)
	if len(prod1) != 1 || e1 {
		t.Fatalf("istore_1 must be the def of slot 1, got %v entry=%v", defOffsets(prod1), e1)
	}
	first := g.Nodes[0]
	_, firstTailEntry := g.ReachingDefinitions(first, 1)
	if firstTailEntry {
		t.Fatal("cat2 tail must not carry an independent live-in identity at method entry")
	}
}

func TestT25C02LinearCat2InterpreterAgrees(t *testing.T) {
	d := auditCFG(t, []byte{OP_LCONST_0, OP_LSTORE_0, OP_ICONST_1, OP_ISTORE_1, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	ret := g.Nodes[len(g.Nodes)-1]
	for _, slot := range []int{0, 1} {
		prod, pe := g.ReachingDefinitions(ret, slot)
		lin, le := LinearCat2Occupancy(g, ret, slot)
		oracle, oe, _ := IndependentReachingOracle(g, ret, slot)
		assertSameDefs(t, "T25-C02 linear slot", prod, lin, pe, le)
		assertSameDefs(t, "T25-C02 oracle slot", prod, oracle, pe, oe)
	}
}

func TestT25DenseJoinTinySetBudget(t *testing.T) {
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_IINC, 0, 1, OP_ILOAD_0, OP_IFNE, 255, 252, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	inc := d.opCodes[d.offsetToOpcodeIndex[2]]
	n := len(g.Nodes)
	g.Work = workbudget.New(nil, workbudget.Limits{MaxSetElementWork: int64(n) + 2})
	s := NewSparseReaching(g)
	_, _, err = s.Definitions(inc, 0)
	if err == nil {
		t.Fatal("dense join with tiny MaxSetElementWork must fail on set work, not complete via queue pops")
	}
	if !workbudget.Is(err) {
		t.Fatalf("want workbudget error, got %v", err)
	}
	if s.Visits() == 0 && g.Work.Used(workbudget.CounterSetElementWork) == 0 {
		t.Fatal("budget must charge actual set work")
	}
}

func TestT25Category2OverlapTinySetBudget(t *testing.T) {
	d := auditCFG(t, []byte{OP_LCONST_0, OP_LSTORE_0, OP_ICONST_1, OP_ISTORE_1, OP_RETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	ret := g.Nodes[len(g.Nodes)-1]
	n := len(g.Nodes)
	g.Work = workbudget.New(nil, workbudget.Limits{MaxSetElementWork: int64(n) + 1})
	s := NewSparseReaching(g)
	_, _, err = s.Definitions(ret, 0)
	if err == nil {
		t.Fatal("cat2 overlap copy/sort must charge set-element work and abort under a tiny cap")
	}
	if !workbudget.Is(err) {
		t.Fatalf("want workbudget error, got %v", err)
	}
}

func TestT25CancelOnSparseEmptyTransitions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	work := workbudget.New(ctx, workbudget.Limits{})
	_, _, err := unionDef(defSet{}, defSet{}, work, nil)
	if err == nil || !workbudget.Is(err) {
		t.Fatalf("empty union must observe cancellation, got %v", err)
	}
	_, err = transferReaching(0, 0, -1, 1, func(reachingDefID) int { return 1 }, defSet{}, work)
	if err == nil || !workbudget.Is(err) {
		t.Fatalf("empty transfer must observe cancellation, got %v", err)
	}
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_ILOAD_0, OP_IRETURN})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	g.Work = work
	s := NewSparseReaching(g)
	if _, _, err := s.Definitions(g.Nodes[2], 0); err == nil || !workbudget.Is(err) {
		t.Fatalf("solver must check cancel, got %v", err)
	}
}

func TestT25BudgetMaxInt64Neighbor(t *testing.T) {
	b := workbudget.New(nil, workbudget.Limits{})
	if err := b.Charge(workbudget.CounterSetElementWork, math.MaxInt64-1); err != nil {
		t.Fatal(err)
	}
	_, _, err := unionDef(singletonDef(0), singletonDef(1), b, nil)
	if err == nil || !workbudget.Is(err) {
		t.Fatalf("union at MaxInt64-1 neighbor must fail before allocating, got %v", err)
	}
	if b.Used(workbudget.CounterSetElementWork) != math.MaxInt64-1 {
		t.Fatalf("failed union mutated used: %d", b.Used(workbudget.CounterSetElementWork))
	}
}
