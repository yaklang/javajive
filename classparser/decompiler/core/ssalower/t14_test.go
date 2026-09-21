package ssalower

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

func applyMoves(moves []Move, state map[VarID]int64) map[VarID]int64 {
	return SerialEval(moves, state)
}

func cloneState(st map[VarID]int64) map[VarID]int64 {
	out := make(map[VarID]int64, len(st))
	for k, v := range st {
		out[k] = v
	}
	return out
}

func TestT14(t *testing.T) {
	t.Run("T14-C01", TestT14_C01_BinarySwap)
	t.Run("T14-C02", TestT14_C02_MultiCycleCopies)
	t.Run("T14-C03", TestT14_C03_CriticalEdgeSplit)
	t.Run("T14-C04", TestT14_C04_ExceptionRegionBoundary)
	t.Run("T14-C05", TestT14_C05_WideAndRefCopies)
}

func TestT14_C01_BinarySwap(t *testing.T) {
	copies := []Copy{{Dst: 1, Src: 2}, {Dst: 2, Src: 1}}
	var n VarID = 10
	moves := Sequentialize(copies, func() VarID { n++; return n })
	st := map[VarID]int64{1: 7, 2: 9}
	got := applyMoves(moves, st)
	want := ParallelEval(copies, st)
	if got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("T14-C01 sequential %v parallel %v moves=%v", got, want, moves)
	}
	if got[1] == got[2] {
		t.Fatal("T14-C01 serialized swap collapsed both variables to one value")
	}
}

func TestT14_C02_MultiCycleCopies(t *testing.T) {
	rng := rand.New(rand.NewSource(20260921))
	cases := [][]Copy{
		{{1, 2}, {2, 3}, {3, 1}},         // 3-cycle
		{{1, 2}, {2, 1}, {3, 4}, {4, 3}}, // two cycles
		{{1, 2}, {3, 2}, {4, 2}},         // fan-out from 2
		{{1, 1}, {2, 3}},                 // self-copy
	}
	for i, copies := range cases {
		var n VarID = 100
		moves := Sequentialize(copies, func() VarID { n++; return n })
		for trial := 0; trial < 20; trial++ {
			st := map[VarID]int64{}
			for v := VarID(1); v <= 6; v++ {
				st[v] = rng.Int63()
			}
			got := applyMoves(moves, st)
			want := ParallelEval(copies, st)
			for _, c := range copies {
				if got[c.Dst] != want[c.Dst] {
					t.Fatalf("T14-C02 case %d trial %d dst %d got %d want %d moves=%v", i, trial, c.Dst, got[c.Dst], want[c.Dst], moves)
				}
			}
		}
	}
}

func TestT14_C03_CriticalEdgeSplit(t *testing.T) {
	t.Run("T14-C03", func(t *testing.T) {
		t.Run("diamond", testT14C03Diamond)
		t.Run("loop-exit", testT14C03LoopExit)
	})
}

func testT14C03Diamond(t *testing.T) {
	// iload_0; ifeq L2; iconst_1; goto L3; L2: iconst_2; L3: ireturn
	code := []byte{
		core.OP_ILOAD_0, core.OP_IFEQ, 0, 7,
		core.OP_ICONST_1, core.OP_GOTO, 0, 4,
		core.OP_ICONST_2,
		core.OP_IRETURN,
	}
	fn, low := ssaDestroy(t, code, "(I)I", nil)
	join, incoming := joinIncoming(t, fn, core.OP_IRETURN)
	if len(incoming) < 2 {
		t.Fatalf("T14-C03 diamond join preds=%d facts:\n%s", len(incoming), fn.Normalize())
	}
	phis := fn.PhisOf(join.ID)
	if len(phis) == 0 {
		t.Fatalf("T14-C03 diamond missing join phi\n%s", fn.Normalize())
	}
	trueEdge, falseEdge := diamondArmEdges(t, fn, incoming)
	pre := preJoinState(fn, join, incoming)
	trueGot := applyMoves(low.MovesOn(trueEdge.ID), cloneState(pre))
	falseGot := applyMoves(low.MovesOn(falseEdge.ID), cloneState(pre))
	trueWant := ParallelEval(phiCopies(fn, trueEdge), pre)
	falseWant := ParallelEval(phiCopies(fn, falseEdge), pre)
	for _, p := range phis {
		dst := PhiDest(p)
		if trueGot[dst] != trueWant[dst] {
			t.Fatalf("T14-C03 true arm dst %d got %d want %d moves=%v", dst, trueGot[dst], trueWant[dst], low.MovesOn(trueEdge.ID))
		}
		if falseGot[dst] != falseWant[dst] {
			t.Fatalf("T14-C03 false arm dst %d got %d want %d moves=%v", dst, falseGot[dst], falseWant[dst], low.MovesOn(falseEdge.ID))
		}
		if trueGot[dst] == falseGot[dst] {
			t.Fatalf("T14-C03 true arm applied false copies (or vice versa): both %d for phi %d", trueGot[dst], dst)
		}
		if trueGot[dst] == falseWant[dst] {
			t.Fatal("T14-C03 taking the true arm applied the false arm's copies")
		}
		if falseGot[dst] == trueWant[dst] {
			t.Fatal("T14-C03 taking the false arm applied the true arm's copies")
		}
	}
	assertEdgeIsolated(t, fn, low, incoming)
	assertSplitsOnlyOnCritical(t, fn, low)
}

func testT14C03LoopExit(t *testing.T) {
	code := loopExitBytecode()
	fn, low := ssaDestroy(t, code, "()I", nil)
	header, incoming := loopHeader(t, fn)
	if len(incoming) < 2 {
		t.Fatalf("T14-C03 loop header preds=%d facts:\n%s", len(incoming), fn.Normalize())
	}
	back, exit := loopBackAndExit(t, fn)
	if back.ID == (methodir.EdgeID{}) || exit.ID == (methodir.EdgeID{}) {
		t.Fatalf("T14-C03 missing backedge/exit back=%s exit=%s edges=%v", back.ID, exit.ID, edgeDump(fn.IR))
	}
	if _, ok := low.SplitFor(back.ID); !ok {
		t.Fatalf("T14-C03 critical backedge was not split (shape-only Destroy)\n%s\n%s", fn.Normalize(), loweredDump(low))
	}
	if s, ok := low.SplitFor(exit.ID); ok && len(s.Moves) > 0 {
		t.Fatalf("T14-C03 exit edge must not carry the backedge split moves: %+v", s)
	}
	pre := preJoinState(fn, header, incoming)
	backGot := applyMoves(low.MovesOn(back.ID), cloneState(pre))
	exitGot := applyMoves(low.MovesOn(exit.ID), cloneState(pre))
	backWant := ParallelEval(phiCopies(fn, back), pre)
	for _, p := range fn.PhisOf(header.ID) {
		dst := PhiDest(p)
		if backGot[dst] != backWant[dst] {
			t.Fatalf("T14-C03 backedge dst %d got %d want %d moves=%v", dst, backGot[dst], backWant[dst], low.MovesOn(back.ID))
		}
		if exitGot[dst] != pre[dst] {
			t.Fatalf("T14-C03 backedge copies executed on the exit edge: dst %d pre %d got %d exitMoves=%v backMoves=%v",
				dst, pre[dst], exitGot[dst], low.MovesOn(exit.ID), low.MovesOn(back.ID))
		}
		if exitGot[dst] == backWant[dst] && backWant[dst] != pre[dst] {
			t.Fatal("T14-C03 loop exit applied backedge copies")
		}
	}
	assertEdgeIsolated(t, fn, low, incoming)
	assertSplitsOnlyOnCritical(t, fn, low)
	s, _ := low.SplitFor(back.ID)
	if s.ID == 0 {
		t.Fatal("T14-C03 SplitID must be unique and non-zero")
	}
	if s.From != back.From || s.To != back.To {
		t.Fatalf("T14-C03 split does not sit only on the backedge: split %+v edge %+v", s, back)
	}
}

func TestT14_C04_ExceptionRegionBoundary(t *testing.T) {
	code, ex, ranges := exceptionTwoRangeBytecode()
	ir, err := methodir.BuildFromBytes(code, ex, methodir.MethodMeta{
		ClassName: "T", Name: "f", Descriptor: "()I", Bytecode: code, IsStatic: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneCoverage(coverageMap(ir))
	throwAt := map[uint16]methodir.Instr{}
	for _, in := range ir.Instrs {
		if in.MayThrow {
			throwAt[in.PC] = in
		}
	}
	fn, err := ssabuild.Build(ir, ssabuild.Options{})
	if err != nil {
		t.Fatal(err)
	}
	low, err := Destroy(fn)
	if err != nil {
		t.Fatal(err)
	}
	after := coverageMap(low.IR)
	if !CoverageMapsEqual(before, after) || !CoverageMapsEqual(before, low.Coverage) {
		t.Fatalf("T14-C04 coverage map changed before=%v after=%v stored=%v", before, after, low.Coverage)
	}
	if !low.ThrowsUnmoved() {
		t.Fatal("T14-C04 ThrowsUnmoved failed after Destroy")
	}
	for _, in := range low.IR.Instrs {
		if !in.MayThrow {
			continue
		}
		orig, ok := throwAt[in.PC]
		if !ok || orig.Opcode != in.Opcode || orig.ID != in.ID {
			t.Fatalf("T14-C04 may-throw op moved from original PC: now pc=%d op=%d id=%d", in.PC, in.Opcode, in.ID)
		}
		inside := false
		for _, r := range ranges {
			if in.PC >= r.Start && in.PC < r.End {
				inside = true
			}
		}
		if !inside {
			t.Fatalf("T14-C04 may-throw PC %d left protected ranges %v", in.PC, ranges)
		}
	}
	for _, a := range low.Assigns {
		if a.Kind != core.EdgeException {
			continue
		}
		from := uint16(a.From)
		inRange := false
		for _, r := range ranges {
			if from >= r.Start && from < r.End {
				inRange = true
			}
		}
		if !inRange {
			t.Fatalf("T14-C04 exception edge from PC %d outside protected ranges", from)
		}
		for _, m := range a.Moves {
			if m.Dst != m.Src {
				t.Fatalf("T14-C04 non-identity copy on exception edge %s: %+v", a.Edge, m)
			}
		}
		for _, m := range low.MovesOn(a.Edge) {
			if m.Dst != m.Src {
				t.Fatalf("T14-C04 MovesOn exception edge %s has real copy %+v", a.Edge, m)
			}
		}
	}
	for _, s := range low.Splits {
		if s.Kind == core.EdgeException {
			t.Fatalf("T14-C04 split inserted on exception edge %+v", s)
		}
		for _, r := range s.Coverage {
			if s.OriginPC < r.Start || s.OriginPC >= r.End {
				t.Fatalf("T14-C04 split origin %d outside coverage [%d,%d)", s.OriginPC, r.Start, r.End)
			}
		}
	}
}

func TestT14_C05_WideAndRefCopies(t *testing.T) {
	copies := []Copy{
		{Dst: 1, Src: 2}, // long-like width recorded on vars
		{Dst: 3, Src: 4},
	}
	vars := []Var{{ID: 1, Width: 2, Kind: "long"}, {ID: 2, Width: 2, Kind: "long"}, {ID: 3, Width: 1, Kind: "ref"}, {ID: 4, Width: 1, Kind: "ref"}}
	var n VarID = 20
	moves := Sequentialize(copies, func() VarID { n++; return n })
	for _, m := range moves {
		if m.Tmp {
			continue
		}
		var dw, sw int
		for _, v := range vars {
			if v.ID == m.Dst {
				dw = v.Width
			}
			if v.ID == m.Src {
				sw = v.Width
			}
		}
		if dw != 0 && sw != 0 && dw != sw {
			t.Fatalf("T14-C05 width split cat2: move %+v", m)
		}
	}
	st := map[VarID]int64{1: 1, 2: 2, 3: 3, 4: 4}
	got := applyMoves(moves, st)
	want := ParallelEval(copies, st)
	if got[1] != want[1] || got[3] != want[3] {
		t.Fatalf("T14-C05 %v vs %v", got, want)
	}
	if vars[0].Width != 2 || vars[0].Kind != "long" {
		t.Fatal("T14-C05 category-2 must stay one semantic var")
	}
}

func ssaDestroy(t *testing.T, code []byte, desc string, ex []*core.ExceptionTableEntry) (*ssabuild.Function, *Lowered) {
	t.Helper()
	ir, err := methodir.BuildFromBytes(code, ex, methodir.MethodMeta{
		ClassName: "T", Name: "f", Descriptor: desc, Bytecode: code, IsStatic: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := ssabuild.Build(ir, ssabuild.Options{})
	if err != nil {
		t.Fatal(err)
	}
	low, err := Destroy(fn)
	if err != nil {
		t.Fatal(err)
	}
	return fn, low
}

func joinIncoming(t *testing.T, fn *ssabuild.Function, opcode int) (methodir.Block, []methodir.Edge) {
	t.Helper()
	var id methodir.InstrID
	found := false
	for _, in := range fn.IR.Instrs {
		if in.Opcode == opcode {
			id = in.ID
			found = true
		}
	}
	if !found {
		t.Fatalf("opcode %d not in IR", opcode)
	}
	bl, ok := fn.IR.BlockOf(id)
	if !ok {
		t.Fatalf("no block for %d", id)
	}
	inc := fn.Incoming(bl.FirstPC)
	if len(inc) == 0 {
		t.Fatalf("no incoming edges to %d", bl.FirstPC)
	}
	return bl, inc
}

func diamondArmEdges(t *testing.T, fn *ssabuild.Function, incoming []methodir.Edge) (methodir.Edge, methodir.Edge) {
	t.Helper()
	var trueE, falseE methodir.Edge
	for _, e := range incoming {
		from, ok := fn.IR.InstrByID(e.From)
		if !ok {
			continue
		}
		switch from.Opcode {
		case core.OP_GOTO:
			trueE = e
		case core.OP_ICONST_2:
			falseE = e
		}
	}
	if trueE.ID == (methodir.EdgeID{}) || falseE.ID == (methodir.EdgeID{}) {
		t.Fatalf("T14-C03 could not identify diamond arms incoming=%s", edgeDump(fn.IR))
	}
	return trueE, falseE
}

func preJoinState(fn *ssabuild.Function, join methodir.Block, incoming []methodir.Edge) map[VarID]int64 {
	st := map[VarID]int64{}
	for _, p := range fn.PhisOf(join.ID) {
		st[PhiDest(p)] = -999999
		for _, op := range p.Operands {
			st[OriginVar(op.Origin)] = int64(OriginVar(op.Origin))
		}
	}
	for _, e := range incoming {
		for _, c := range phiCopies(fn, e) {
			if _, ok := st[c.Src]; !ok {
				st[c.Src] = int64(c.Src)
			}
			if _, ok := st[c.Dst]; !ok {
				st[c.Dst] = -999999
			}
		}
	}
	return st
}

func assertEdgeIsolated(t *testing.T, fn *ssabuild.Function, low *Lowered, incoming []methodir.Edge) {
	t.Helper()
	pre := preJoinState(fn, func() methodir.Block {
		bl, _ := fn.IR.BlockOf(incoming[0].To)
		return bl
	}(), incoming)
	for i, e := range incoming {
		got := applyMoves(low.MovesOn(e.ID), cloneState(pre))
		want := ParallelEval(phiCopies(fn, e), pre)
		for _, c := range phiCopies(fn, e) {
			if got[c.Dst] != want[c.Dst] {
				t.Fatalf("T14-C03 edge %s dst %d got %d want %d", e.ID, c.Dst, got[c.Dst], want[c.Dst])
			}
		}
		for j, other := range incoming {
			if i == j {
				continue
			}
			otherWant := ParallelEval(phiCopies(fn, other), pre)
			for _, c := range phiCopies(fn, other) {
				if c.Dst == 0 {
					continue
				}
				mine := phiCopies(fn, e)
				appliesMine := false
				for _, mc := range mine {
					if mc.Dst == c.Dst && mc.Src == c.Src {
						appliesMine = true
					}
				}
				if appliesMine {
					continue
				}
				if len(mine) == 0 && got[c.Dst] == otherWant[c.Dst] && otherWant[c.Dst] != pre[c.Dst] {
					t.Fatalf("T14-C03 edge %s applied sibling %s copy dst %d", e.ID, other.ID, c.Dst)
				}
				if got[c.Dst] == otherWant[c.Dst] && got[c.Dst] != pre[c.Dst] && got[c.Dst] != want[c.Dst] {
					t.Fatalf("T14-C03 edge %s leaked sibling copy dst %d value %d", e.ID, c.Dst, got[c.Dst])
				}
			}
		}
	}
}

func assertSplitsOnlyOnCritical(t *testing.T, fn *ssabuild.Function, low *Lowered) {
	t.Helper()
	succN := map[methodir.InstrID]int{}
	predN := map[methodir.InstrID]int{}
	for _, e := range fn.IR.Edges {
		succN[e.From]++
		predN[e.To]++
	}
	seen := map[int]methodir.EdgeID{}
	for _, s := range low.Splits {
		if s.ID <= 0 {
			t.Fatalf("T14-C03 SplitID must be unique positive, got %d", s.ID)
		}
		if prev, ok := seen[s.ID]; ok {
			t.Fatalf("T14-C03 SplitID %d reused for %s and %s", s.ID, prev, s.Edge)
		}
		seen[s.ID] = s.Edge
		var e methodir.Edge
		for _, x := range fn.IR.Edges {
			if x.ID == s.Edge {
				e = x
			}
		}
		if succN[e.From] <= 1 || predN[e.To] <= 1 {
			t.Fatalf("T14-C03 split on non-critical edge %s succ=%d pred=%d", s.Edge, succN[e.From], predN[e.To])
		}
		if e.Kind == core.EdgeException {
			t.Fatalf("T14-C03 split on exception edge %s", s.Edge)
		}
		if len(s.Moves) == 0 {
			t.Fatalf("T14-C03 split %d has no Sequentialize moves", s.ID)
		}
		for _, o := range low.Splits {
			if o.ID != s.ID && o.Edge == s.Edge {
				t.Fatalf("T14-C03 split identity %d is not unique to edge %s", s.ID, s.Edge)
			}
		}
	}
	for _, e := range fn.IR.Edges {
		if e.Kind == core.EdgeException {
			continue
		}
		copies := phiCopies(fn, e)
		need := false
		for _, c := range copies {
			if c.Dst != c.Src {
				need = true
				break
			}
		}
		if succN[e.From] > 1 && predN[e.To] > 1 && need {
			if _, ok := low.SplitFor(e.ID); !ok {
				t.Fatalf("T14-C03 missing split on critical edge %s copies=%v", e.ID, copies)
			}
		}
	}
}

func loopExitBytecode() []byte {
	var c []byte
	c = append(c, core.OP_ICONST_0, core.OP_ISTORE_1)
	header := len(c)
	c = append(c, core.OP_IINC, 1, 1)
	c = append(c, core.OP_ILOAD_1, core.OP_BIPUSH, 3)
	ifPC := len(c)
	c = append(c, core.OP_IF_ICMPLT, 0, 0)
	c = append(c, core.OP_ILOAD_1, core.OP_IRETURN)
	off := int16(header - ifPC)
	binary.BigEndian.PutUint16(c[ifPC+1:], uint16(off))
	return c
}

func loopHeader(t *testing.T, fn *ssabuild.Function) (methodir.Block, []methodir.Edge) {
	t.Helper()
	var header methodir.Block
	var incoming []methodir.Edge
	for _, b := range fn.IR.Blocks {
		inc := fn.Incoming(b.FirstPC)
		if len(inc) >= 2 {
			hasPhi := len(fn.PhisOf(b.ID)) > 0
			if hasPhi {
				header = b
				incoming = inc
				break
			}
		}
	}
	if len(incoming) < 2 {
		t.Fatalf("T14-C03 no loop-header join\n%s", fn.Normalize())
	}
	return header, incoming
}

func loopBackAndExit(t *testing.T, fn *ssabuild.Function) (methodir.Edge, methodir.Edge) {
	t.Helper()
	var ifID methodir.InstrID
	for _, in := range fn.IR.Instrs {
		if in.Opcode == core.OP_IF_ICMPLT {
			ifID = in.ID
		}
	}
	var back, exit methodir.Edge
	for _, e := range fn.IR.Edges {
		if e.From != ifID {
			continue
		}
		switch e.Kind {
		case core.EdgeTaken:
			back = e
		case core.EdgeFallthrough:
			exit = e
		}
	}
	return back, exit
}

func exceptionTwoRangeBytecode() ([]byte, []*core.ExceptionTableEntry, []Range) {
	var c []byte
	c = append(c, core.OP_ICONST_0, core.OP_ISTORE_0)
	a0 := len(c)
	c = append(c, core.OP_ICONST_1, core.OP_ISTORE_0, core.OP_INVOKESTATIC, 0, 1)
	b0 := len(c)
	c = append(c, core.OP_ICONST_2, core.OP_ISTORE_0, core.OP_INVOKESTATIC, 0, 1)
	endB := len(c)
	c = append(c, core.OP_RETURN)
	h := len(c)
	c = append(c, core.OP_ASTORE_1, core.OP_ILOAD_0, core.OP_IRETURN)
	ex := []*core.ExceptionTableEntry{
		{StartPc: uint16(a0), EndPc: uint16(b0), HandlerPc: uint16(h), CatchType: 1},
		{StartPc: uint16(b0), EndPc: uint16(endB), HandlerPc: uint16(h), CatchType: 1},
	}
	ranges := []Range{
		{Start: uint16(a0), End: uint16(b0), Handler: uint16(h), CatchType: 1, Order: 0},
		{Start: uint16(b0), End: uint16(endB), Handler: uint16(h), CatchType: 1, Order: 1},
	}
	return c, ex, ranges
}

func edgeDump(ir *methodir.MethodIR) string {
	s := ""
	for _, e := range ir.Edges {
		s += fmt.Sprintf("%s %d->%d kind=%s; ", e.ID, e.From, e.To, methodir.KindName(e.Kind))
	}
	return s
}

func loweredDump(l *Lowered) string {
	s := ""
	for _, a := range l.Assigns {
		s += fmt.Sprintf("assign %s split=%v id=%d moves=%v\n", a.Edge, a.Split, a.SplitID, a.Moves)
	}
	for _, sp := range l.Splits {
		s += fmt.Sprintf("split %d edge=%s moves=%v\n", sp.ID, sp.Edge, sp.Moves)
	}
	return s
}
