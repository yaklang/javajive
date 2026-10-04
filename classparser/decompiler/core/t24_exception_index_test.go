package core

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestT24C01OrderedEdgesMatchReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20260921))
	for trial := 0; trial < 40; trial++ {
		nPC := 16 + rng.Intn(48)
		throwPCs := make([]uint16, 0, nPC)
		for pc := 0; pc < nPC; pc++ {
			if rng.Intn(3) != 0 {
				throwPCs = append(throwPCs, uint16(pc))
			}
		}
		nTab := 1 + rng.Intn(12)
		table := make([]HandlerRange, nTab)
		for i := range table {
			start := uint16(rng.Intn(nPC))
			end := start + uint16(1+rng.Intn(8))
			if int(end) > nPC {
				end = uint16(nPC)
			}
			catch := uint16(1 + rng.Intn(4))
			if rng.Intn(6) == 0 {
				catch = 0
			}
			table[i] = HandlerRange{StartPc: start, EndPc: end, HandlerPc: uint16(100 + i), CatchType: catch}
		}
		slow, _, err := SlowExceptionEdges(throwPCs, table, nil)
		if err != nil {
			t.Fatal(err)
		}
		fast, _, err := IndexedExceptionEdges(throwPCs, table, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !exceptionEdgesEqual(slow, fast) {
			t.Fatalf("T24-C01 trial %d mismatch %s", trial, formatEdgeMismatch(slow, fast))
		}
	}
}

func TestT24C02HalfOpenBounds(t *testing.T) {
	table := []HandlerRange{
		{StartPc: 10, EndPc: 20, HandlerPc: 50, CatchType: 1},
		{StartPc: 20, EndPc: 20, HandlerPc: 51, CatchType: 2}, // zero-length illegal
		{StartPc: 20, EndPc: 19, HandlerPc: 52, CatchType: 3}, // inverted
		{StartPc: 20, EndPc: 30, HandlerPc: 53, CatchType: 4},
	}
	pcs := []uint16{9, 10, 19, 20, 29, 30}
	got, _, err := IndexedExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	slow, _, err := SlowExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !exceptionEdgesEqual(slow, got) {
		t.Fatalf("T24-C02 slow/fast diverge %s", formatEdgeMismatch(slow, got))
	}
	covers := map[uint16][]uint16{}
	for _, e := range got {
		covers[e.ThrowPc] = append(covers[e.ThrowPc], e.HandlerPc)
	}
	if !reflect.DeepEqual(covers[9], []uint16(nil)) {
		t.Fatalf("pc 9 before start: %v", covers[9])
	}
	if !reflect.DeepEqual(covers[10], []uint16{50}) {
		t.Fatalf("start inclusive: %v", covers[10])
	}
	if !reflect.DeepEqual(covers[19], []uint16{50}) {
		t.Fatalf("last in range: %v", covers[19])
	}
	if !reflect.DeepEqual(covers[20], []uint16{53}) {
		t.Fatalf("end exclusive and shared start: %v", covers[20])
	}
	if !reflect.DeepEqual(covers[29], []uint16{53}) {
		t.Fatalf("second range: %v", covers[29])
	}
	if covers[30] != nil {
		t.Fatalf("end exclusive of second range: %v", covers[30])
	}
	for _, e := range got {
		if e.HandlerPc == 51 || e.HandlerPc == 52 {
			t.Fatalf("zero-length/inverted range leaked: %+v", e)
		}
	}
}

func TestT24C03CatchAllShadow(t *testing.T) {
	table := []HandlerRange{
		{StartPc: 0, EndPc: 10, HandlerPc: 40, CatchType: 1}, // typed
		{StartPc: 0, EndPc: 10, HandlerPc: 41, CatchType: 0}, // catch-all
		{StartPc: 0, EndPc: 10, HandlerPc: 41, CatchType: 2}, // later, shared target
		{StartPc: 0, EndPc: 10, HandlerPc: 42, CatchType: 3},
	}
	got, _, err := IndexedExceptionEdges([]uint16{0, 9}, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	slow, _, err := SlowExceptionEdges([]uint16{0, 9}, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !exceptionEdgesEqual(slow, got) {
		t.Fatalf("T24-C03 %s", formatEdgeMismatch(slow, got))
	}
	want := []IndexedExceptionEdge{
		{0, 40, 1, 0}, {0, 41, 0, 1},
		{9, 40, 1, 0}, {9, 41, 0, 1},
	}
	if !exceptionEdgesEqual(got, want) {
		t.Fatalf("T24-C03 catch-all must keep prior typed handlers and drop later rows: %v", got)
	}
}

func TestT24C04SparseScanWorkDrops(t *testing.T) {
	for _, n := range []int{32, 64, 128} {
		table := make([]HandlerRange, n)
		throwPCs := make([]uint16, 0, n*4)
		for i := 0; i < n; i++ {
			start := uint16(i * 8)
			table[i] = HandlerRange{StartPc: start, EndPc: start + 4, HandlerPc: uint16(1000 + i), CatchType: 1}
			for pc := start; pc < start+8; pc++ {
				throwPCs = append(throwPCs, pc)
			}
		}
		slow, ss, err := SlowExceptionEdges(throwPCs, table, nil)
		if err != nil {
			t.Fatal(err)
		}
		fast, fs, err := IndexedExceptionEdges(throwPCs, table, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !exceptionEdgesEqual(slow, fast) {
			t.Fatalf("T24-C04 N=%d output differs", n)
		}
		if fs.CandidateScans >= ss.CandidateScans {
			t.Fatalf("T24-C04 N=%d indexed scans %d not below table scans %d", n, fs.CandidateScans, ss.CandidateScans)
		}
		// CandidateScans drop is not a proof that total cost (IndexWork+edges) is linear.
		if fs.EmittedEdges != ss.EmittedEdges {
			t.Fatalf("T24-C04 edge count %d vs %d", fs.EmittedEdges, ss.EmittedEdges)
		}
		t.Logf("T24-C04 N=%d slow_scans=%d indexed_scans=%d edges=%d", n, ss.CandidateScans, fs.CandidateScans, fs.EmittedEdges)
	}
}

func TestT24C05DenseFanoutBudget(t *testing.T) {
	n, throws := 40, 40
	table := make([]HandlerRange, n)
	pcs := make([]uint16, throws)
	for i := 0; i < n; i++ {
		table[i] = HandlerRange{StartPc: 0, EndPc: uint16(throws), HandlerPc: uint16(200 + i), CatchType: uint16(i + 1)}
	}
	for i := 0; i < throws; i++ {
		pcs[i] = uint16(i)
	}
	slow, ss, err := SlowExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	fast, fs, err := IndexedExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !exceptionEdgesEqual(slow, fast) {
		t.Fatal("T24-C05 dense output must match reference")
	}
	// Overlapping handlers produce Θ(throws*table) edges; the index cannot be sublinear in output.
	if fs.EmittedEdges != uint64(n*throws) {
		t.Fatalf("T24-C05 expected %d edges, got %d", n*throws, fs.EmittedEdges)
	}
	if fs.CandidateScans < uint64(n*throws)/2 {
		t.Fatalf("T24-C05 must not pretend dense work is sparse: scans=%d", fs.CandidateScans)
	}
	// Tiny MaxGraphEdges with unlimited MaxGraphScans must abort before materializing Θ(n*throws) edges.
	limit := int64(n) // too small for even two PCs' worth of edges
	budget := workbudget.New(nil, workbudget.Limits{MaxGraphEdges: limit})
	got, gs, err := IndexedExceptionEdges(pcs, table, budget)
	if err == nil {
		t.Fatal("T24-C05 expected analysis_budget_exceeded")
	}
	if !workbudget.Is(err) {
		t.Fatalf("T24-C05 want workbudget.Error, got %v", err)
	}
	if int64(len(got)) > limit {
		t.Fatalf("T24-C05 retained %d edges after MaxGraphEdges=%d (full fanout would be %d)", len(got), limit, n*throws)
	}
	if int64(gs.EmittedEdges) > limit {
		t.Fatalf("T24-C05 emitted %d edges after budget %d", gs.EmittedEdges, limit)
	}
	if int64(gs.PeakBuffered) > limit+1 {
		t.Fatalf("T24-C05 PeakBuffered %d materialized a full PC of %d handlers under MaxGraphEdges=%d", gs.PeakBuffered, n, limit)
	}
	if budget.Used(workbudget.CounterGraphEdges) > limit {
		t.Fatalf("T24-C05 graph_edges used %d > limit %d", budget.Used(workbudget.CounterGraphEdges), limit)
	}
	t.Logf("T24-C05 dense edges=%d slow_scans=%d indexed_scans=%d retained=%d peak=%d budget_err=%v", fs.EmittedEdges, ss.CandidateScans, fs.CandidateScans, len(got), gs.PeakBuffered, err)
}

func TestT24C04RepeatScanMeasurements(t *testing.T) {
	n := 64
	table := make([]HandlerRange, n)
	throwPCs := make([]uint16, 0, n*4)
	for i := 0; i < n; i++ {
		start := uint16(i * 8)
		table[i] = HandlerRange{StartPc: start, EndPc: start + 4, HandlerPc: uint16(1000 + i), CatchType: 1}
		for pc := start; pc < start+8; pc++ {
			throwPCs = append(throwPCs, pc)
		}
	}
	for i := 0; i < 10; i++ {
		slow, ss, err := SlowExceptionEdges(throwPCs, table, nil)
		if err != nil {
			t.Fatal(err)
		}
		fast, fs, err := IndexedExceptionEdges(throwPCs, table, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !exceptionEdgesEqual(slow, fast) || fs.CandidateScans >= ss.CandidateScans {
			t.Fatalf("repeat %d failed scans slow=%d fast=%d", i, ss.CandidateScans, fs.CandidateScans)
		}
		t.Logf("repeat=%d slow_scans=%d indexed_scans=%d edges=%d", i, ss.CandidateScans, fs.CandidateScans, fs.EmittedEdges)
	}
}

func TestT24IndexMatchesProductionCFG(t *testing.T) {
	d := auditCFG(t, []byte{OP_ICONST_0, OP_ISTORE_0, OP_ICONST_1, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_ICONST_2, OP_ISTORE_0, OP_INVOKESTATIC, 0, 1, OP_RETURN, OP_ASTORE_1, OP_ILOAD_0, OP_IRETURN})
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 2, EndPc: 12, HandlerPc: 13, CatchType: 1}}
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	prod := ProductionExceptionEdges(g)
	ranges := ExceptionTableAsRanges(d.ExceptionTable)
	slow, _, err := SlowExceptionEdges(MayThrowPCs(g), ranges, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prod) != len(slow) {
		t.Fatalf("production %v vs slow oracle %v", prod, slow)
	}
	for i := range prod {
		if prod[i].ThrowPc != slow[i].ThrowPc || prod[i].HandlerPc != slow[i].HandlerPc || prod[i].HandlerOrder != slow[i].HandlerOrder {
			t.Fatalf("edge %d production %+v slow %+v", i, prod[i], slow[i])
		}
	}
}

func TestT24ProductionConstructorUsesIndex(t *testing.T) {
	t.Run("T24-C04", func(t *testing.T) {
		n := 32
		code := []byte{OP_ICONST_0, OP_ISTORE_0}
		for i := 0; i < n; i++ {
			code = append(code, OP_GETSTATIC, 0, 1)
		}
		code = append(code, OP_RETURN)
		d := auditCFG(t, code)
		d.ExceptionTable = make([]*ExceptionTableEntry, n)
		for i := 0; i < n; i++ {
			pc := uint16(2 + i*3)
			d.ExceptionTable[i] = &ExceptionTableEntry{StartPc: pc, EndPc: pc + 3, HandlerPc: uint16(len(code) - 1), CatchType: 1}
		}
		g, err := d.buildSemanticCFG()
		if err != nil {
			t.Fatal(err)
		}
		ranges := ExceptionTableAsRanges(d.ExceptionTable)
		_, ss, err := SlowExceptionEdges(MayThrowPCs(g), ranges, nil)
		if err != nil {
			t.Fatal(err)
		}
		if g.GraphScans == 0 {
			t.Fatal("production constructor recorded no graph_scans")
		}
		if uint64(g.GraphScans) >= ss.CandidateScans {
			t.Fatalf("production still table-scans: graph_scans=%d slow=%d", g.GraphScans, ss.CandidateScans)
		}
	})
}

func TestT24UnsortedDuplicatePCsMatchSlowOracle(t *testing.T) {
	table := []HandlerRange{
		{StartPc: 0, EndPc: 10, HandlerPc: 40, CatchType: 1},
		{StartPc: 10, EndPc: 20, HandlerPc: 41, CatchType: 2},
		{StartPc: 5, EndPc: 15, HandlerPc: 42, CatchType: 3},
	}
	pcs := []uint16{19, 0, 10, 10, 5, 19, 9, 5}
	slow, _, err := SlowExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	fast, _, err := IndexedExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !exceptionEdgesEqual(slow, fast) {
		t.Fatalf("unsorted/duplicate PCs diverge %s", formatEdgeMismatch(slow, fast))
	}
	if len(fast) == 0 {
		t.Fatal("expected covering edges")
	}
}

func TestT24SameHandlerBoundaryEvents(t *testing.T) {
	table := []HandlerRange{
		{StartPc: 0, EndPc: 10, HandlerPc: 50, CatchType: 1},
		{StartPc: 10, EndPc: 20, HandlerPc: 51, CatchType: 2},
		{StartPc: 10, EndPc: 10, HandlerPc: 52, CatchType: 3}, // zero-length
		{StartPc: 20, EndPc: 20, HandlerPc: 53, CatchType: 4},
	}
	pcs := []uint16{9, 10, 19, 20}
	slow, _, err := SlowExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	fast, _, err := IndexedExceptionEdges(pcs, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !exceptionEdgesEqual(slow, fast) {
		t.Fatalf("boundary events %s", formatEdgeMismatch(slow, fast))
	}
	covers := map[uint16][]uint16{}
	for _, e := range fast {
		covers[e.ThrowPc] = append(covers[e.ThrowPc], e.HandlerPc)
	}
	if !reflect.DeepEqual(covers[9], []uint16{50}) || !reflect.DeepEqual(covers[10], []uint16{51}) {
		t.Fatalf("same-PC remove-before-add failed: %v", covers)
	}
	if covers[20] != nil {
		t.Fatalf("end-exclusive zero-length leaked: %v", covers[20])
	}
}

func TestT24JSRInlinedCloneIdentities(t *testing.T) {
	// Two throwing ops share a PC (JSR-inlined clones keep original offsets until remap;
	// production must emit from each *OpCode, not collapse by PC).
	code := []byte{OP_GETSTATIC, 0, 1, OP_GETSTATIC, 0, 1, OP_RETURN, OP_ASTORE_0, OP_RETURN}
	d := auditCFG(t, code)
	first := d.opCodes[d.offsetToOpcodeIndex[0]]
	second := d.opCodes[d.offsetToOpcodeIndex[3]]
	second.CurrentOffset = first.CurrentOffset
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 6, HandlerPc: 7, CatchType: 1}}
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	var fromFirst, fromSecond int
	for _, e := range g.Edges {
		if e.Kind != EdgeException {
			continue
		}
		if e.From == first {
			fromFirst++
		}
		if e.From == second {
			fromSecond++
		}
	}
	if fromFirst != 1 || fromSecond != 1 {
		t.Fatalf("cloned same-PC throw ops need distinct edges, first=%d second=%d", fromFirst, fromSecond)
	}
	slow, _, err := SlowExceptionEdges(MayThrowPCs(g), ExceptionTableAsRanges(d.ExceptionTable), nil)
	if err != nil {
		t.Fatal(err)
	}
	prod := ProductionExceptionEdges(g)
	if len(prod) != len(slow) {
		t.Fatalf("production vs slow on cloned PCs %s", formatEdgeMismatch(slow, prod))
	}
	for i := range prod {
		if prod[i].ThrowPc != slow[i].ThrowPc || prod[i].HandlerPc != slow[i].HandlerPc || prod[i].HandlerOrder != slow[i].HandlerOrder {
			t.Fatalf("cloned edge %d production %+v slow %+v", i, prod[i], slow[i])
		}
	}
}

func TestT24DenseFanoutAbortsBeforeFullMaterialize(t *testing.T) {
	n, throws := 50, 50
	table := make([]HandlerRange, n)
	pcs := make([]uint16, throws)
	for i := 0; i < n; i++ {
		table[i] = HandlerRange{StartPc: 0, EndPc: uint16(throws), HandlerPc: uint16(400 + i), CatchType: uint16(i + 1)}
	}
	for i := 0; i < throws; i++ {
		pcs[i] = uint16(i)
	}
	full := int64(n * throws)
	limit := int64(7)
	budget := workbudget.New(nil, workbudget.Limits{MaxGraphEdges: limit, MaxGraphScans: 0})
	var retained int
	var visits int
	stats, err := StreamIndexedExceptionEdges(pcs, table, budget, func(e IndexedExceptionEdge) error {
		visits++
		if err := budget.Charge(workbudget.CounterGraphEdges, 1); err != nil {
			return err
		}
		retained++
		return nil
	})
	if err == nil {
		t.Fatal("dense fanout with MaxGraphEdges=7 must abort")
	}
	if !workbudget.Is(err) {
		t.Fatalf("want workbudget error, got %v", err)
	}
	if int64(retained) > limit {
		t.Fatalf("retained %d edges; full fanout is %d", retained, full)
	}
	if int64(visits) > limit+1 {
		t.Fatalf("visited/emitted %d allocations after limit %d — full intermediate was built", visits, limit)
	}
	if int64(stats.PeakBuffered) > limit+1 {
		t.Fatalf("PeakBuffered %d under MaxGraphEdges=%d", stats.PeakBuffered, limit)
	}
	if int64(stats.EmittedEdges) > limit {
		t.Fatalf("EmittedEdges %d after abort (limit %d)", stats.EmittedEdges, limit)
	}
	if budget.Used(workbudget.CounterGraphEdges) > limit {
		t.Fatalf("graph_edges used %d", budget.Used(workbudget.CounterGraphEdges))
	}
}

func TestT24ProductionDenseFanoutTinyMaxGraphEdges(t *testing.T) {
	n := 24
	code := []byte{OP_ICONST_0, OP_ISTORE_0}
	for i := 0; i < n; i++ {
		code = append(code, OP_GETSTATIC, 0, 1)
	}
	code = append(code, OP_RETURN, OP_ASTORE_0, OP_RETURN)
	d := auditCFG(t, code)
	handler := uint16(len(code) - 2)
	d.ExceptionTable = make([]*ExceptionTableEntry, n)
	for i := 0; i < n; i++ {
		d.ExceptionTable[i] = &ExceptionTableEntry{StartPc: 0, EndPc: uint16(len(code) - 2), HandlerPc: handler, CatchType: uint16(i + 1)}
	}
	// Normal edges are O(n); exception fanout is n×n. Cap just above the fallthrough spine.
	limit := int64(n + 8)
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphEdges: limit, MaxGraphScans: 0})
	g, err := d.buildSemanticCFG()
	if err == nil {
		t.Fatal("production dense exception fanout must trip MaxGraphEdges")
	}
	if !workbudget.Is(err) {
		t.Fatalf("want workbudget error, got %v", err)
	}
	if g == nil {
		t.Fatal("partial graph should be returned")
	}
	if int64(len(g.Edges)) > limit {
		t.Fatalf("production retained %d edges after MaxGraphEdges=%d", len(g.Edges), limit)
	}
	if int64(g.IndexPeak) > limit+1 {
		t.Fatalf("IndexPeak %d under MaxGraphEdges=%d (full throw×handler was buffered)", g.IndexPeak, limit)
	}
	if int64(g.IndexEmitted) > limit {
		t.Fatalf("IndexEmitted %d after abort", g.IndexEmitted)
	}
	if d.Work.Used(workbudget.CounterGraphEdges) > limit {
		t.Fatalf("graph_edges used %d", d.Work.Used(workbudget.CounterGraphEdges))
	}
}

func TestT24IndexWorkChargedToCanonicalWork(t *testing.T) {
	table := []HandlerRange{{StartPc: 0, EndPc: 4, HandlerPc: 8, CatchType: 1}}
	pcs := []uint16{0, 1, 2, 3}
	budget := workbudget.New(nil, workbudget.Limits{})
	_, stats, err := IndexedExceptionEdges(pcs, table, budget)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Used(workbudget.CounterGraphScans) == 0 {
		t.Fatal("event insert/sort/apply must charge graph_scans on canonical Work")
	}
	if stats.IndexWork == 0 {
		t.Fatal("IndexWork must count event construction")
	}
	if budget.Used(workbudget.CounterGraphScans) < int64(stats.IndexWork) {
		t.Fatalf("Work.graph_scans=%d < IndexWork=%d", budget.Used(workbudget.CounterGraphScans), stats.IndexWork)
	}
	code := []byte{OP_GETSTATIC, 0, 1, OP_RETURN, OP_ASTORE_0, OP_RETURN}
	d := auditCFG(t, code)
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 3, HandlerPc: 4, CatchType: 1}}
	d.Work = workbudget.New(nil, workbudget.Limits{})
	g, err := d.buildSemanticCFG()
	if err != nil {
		t.Fatal(err)
	}
	if d.Work.Used(workbudget.CounterGraphScans) == 0 || g.IndexWork == 0 {
		t.Fatalf("production constructor did not charge index work: used=%d IndexWork=%d", d.Work.Used(workbudget.CounterGraphScans), g.IndexWork)
	}
}

func TestT24BudgetMaxInt64Neighbor(t *testing.T) {
	b := workbudget.New(nil, workbudget.Limits{})
	if err := b.Charge(workbudget.CounterGraphEdges, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := b.Charge(workbudget.CounterGraphEdges, 1); err == nil || !workbudget.Is(err) {
		t.Fatalf("MaxInt64 neighbor must reject, got %v", err)
	}
	if b.Used(workbudget.CounterGraphEdges) != math.MaxInt64 {
		t.Fatalf("overflow mutated used: %d", b.Used(workbudget.CounterGraphEdges))
	}
	table := []HandlerRange{{StartPc: 0, EndPc: 1, HandlerPc: 2, CatchType: 1}}
	near := workbudget.New(nil, workbudget.Limits{})
	if err := near.Charge(workbudget.CounterGraphEdges, math.MaxInt64-1); err != nil {
		t.Fatal(err)
	}
	_, _, err := IndexedExceptionEdges([]uint16{0}, table, near)
	if err == nil || !workbudget.Is(err) {
		t.Fatalf("emit at MaxInt64-1 neighbor must fail, got %v", err)
	}
}
