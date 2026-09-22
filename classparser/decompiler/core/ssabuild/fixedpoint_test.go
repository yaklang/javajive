package ssabuild

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

func TestPermanentEntryAndReboundLoopUses(t *testing.T) {
	// Entry is also the loop header: iload0; ineg; istore0; goto entry.
	ir := irOf(t, []byte{core.OP_ILOAD_0, core.OP_INEG, core.OP_ISTORE_0, core.OP_GOTO, 0xff, 0xfd}, "(I)V", nil)
	fn := ssaOf(t, ir)
	var phi *Phi
	for i := range fn.Phis {
		if fn.Phis[i].Block == fn.Blocks[0].ID && fn.Phis[i].Slot == (SlotKey{Local: true, Index: 0}) {
			phi = &fn.Phis[i]
		}
	}
	if phi == nil || len(phi.Operands) != 2 {
		t.Fatalf("entry/backedge phi missing: %+v", fn.Phis)
	}
	haveParam, haveNeg := false, false
	for _, op := range phi.Operands {
		haveParam = haveParam || op.Edge.Kind == EntryEdgeKind && op.Origin.Kind == OriginParam
		haveNeg = haveNeg || op.Origin.Kind == OriginInstr && op.Origin.PC == 1
	}
	if !haveParam || !haveNeg {
		t.Fatalf("lost entry or backedge: %+v", phi)
	}
	for _, r := range fn.Instructions {
		if r.PC == 1 && (len(r.Uses) != 1 || r.Uses[0].Kind != OriginPhi || r.Uses[0].PC != 0) {
			t.Fatalf("negation retained stale first-visit operands: %+v", r)
		}
	}
	if fn.Work > 50 {
		t.Fatalf("loop did not stabilize: %d", fn.Work)
	}
}

func TestEntryContributionParticipatesInTypeJoin(t *testing.T) {
	// A backedge cannot add a stack value to the method-entry empty stack.
	ir := irOf(t, []byte{core.OP_ICONST_0, core.OP_GOTO, 0xff, 0xff}, "()V", nil)
	if _, err := Build(ir, Options{}); err == nil {
		t.Fatal("entry stack contribution discarded")
	}
}

func TestInitialFrameAllParametersAndErrors(t *testing.T) {
	ir := irOf(t, []byte{core.OP_RETURN}, "(JJJDI)V", nil)
	fn := ssaOf(t, ir)
	if len(fn.Blocks[0].In.Locals) != 9 || fn.Blocks[0].In.Locals[8].Kind != frametransfer.Int {
		t.Fatalf("parameters truncated: %s", fn.Blocks[0].In.Canonical())
	}
	ir.Descriptor = "(broken"
	if _, err := Build(ir, Options{}); err == nil {
		t.Fatal("malformed descriptor ignored")
	}
}

func TestUnreachablePredecessorAndParallelEdges(t *testing.T) {
	ir := irOf(t, []byte{core.OP_NOP, core.OP_NOP, core.OP_RETURN}, "(I)V", nil)
	edge := func(from, to methodir.InstrID, key int32) methodir.Edge {
		id := methodir.EdgeID{From: from, To: to, Kind: core.EdgeCase, CaseValue: key}
		return methodir.Edge{ID: id, From: from, To: to, Kind: core.EdgeCase, CaseValue: key}
	}
	ir.Edges = []methodir.Edge{edge(0, 2, 1), edge(0, 2, 2), edge(1, 2, 3)}
	fn := ssaOf(t, ir)
	if fn.Blocks[1].Reachable || len(fn.Phis) != 0 {
		t.Fatalf("unreachable predecessor introduced a definition: %+v", fn.Phis)
	}
	for _, e := range ir.Edges[:2] {
		if _, ok := fn.EdgeStates[e.ID]; !ok {
			t.Fatalf("parallel edge lost: %v", e.ID)
		}
	}
	if _, ok := fn.EdgeStates[ir.Edges[2].ID]; ok {
		t.Fatal("fabricated unreachable edge contribution")
	}
}

// definitionSets is an independent synchronous reaching-definitions oracle.
// It does not use Build, transferOrigins, join helpers or SSA phi creation.
func definitionSets(ir *methodir.MethodIR, slots int) [][][]bool {
	n := len(ir.Blocks)
	out := make([][][]bool, n)
	pcIndex := map[methodir.InstrID]int{}
	for i, b := range ir.Blocks {
		pcIndex[methodir.InstrID(b.FirstPC)] = i
	}
	for rounds := 0; rounds < 4*n*n+10; rounds++ {
		next := make([][][]bool, n)
		makeState := func() [][]bool {
			s := make([][]bool, slots)
			for i := range s {
				s[i] = make([]bool, n+slots)
			}
			return s
		}
		next[0] = makeState()
		for j := 0; j < slots; j++ {
			next[0][j][j] = true
		}
		for _, e := range ir.Edges {
			from, to := pcIndex[e.From], pcIndex[e.To]
			if out[from] == nil {
				continue
			}
			if next[to] == nil {
				next[to] = makeState()
			}
			ins := ir.Instrs[from]
			for j := 0; j < slots; j++ {
				if ins.Opcode == core.OP_IINC && ins.Local == j {
					next[to][j][slots+from] = true
					continue
				}
				for k, v := range out[from][j] {
					next[to][j][k] = next[to][j][k] || v
				}
			}
		}
		if reflect.DeepEqual(next, out) {
			return next
		}
		out = next
	}
	panic("oracle did not converge")
}

func expandedSSA(t *testing.T, fn *Function, slots int) [][][]bool {
	t.Helper()
	n := len(fn.Blocks)
	pcIndex := map[uint16]int{}
	for i, b := range fn.Blocks {
		pcIndex[b.First] = i
	}
	sets := map[Origin][]bool{}
	ensure := func(o Origin) []bool {
		if v, ok := sets[o]; ok {
			return v
		}
		v := make([]bool, n+slots)
		if o.Kind == OriginParam {
			v[o.Slot] = true
		}
		if o.Kind == OriginInstr && o.Slot >= 0 {
			v[slots+pcIndex[o.PC]] = true
		}
		sets[o] = v
		return v
	}
	for _, b := range fn.Blocks {
		for _, o := range b.InOrig {
			ensure(o)
		}
	}
	for _, p := range fn.Phis {
		for _, o := range p.Operands {
			ensure(o.Origin)
		}
	}
	for rounds := 0; rounds < n*n+10; rounds++ {
		changed := false
		for _, p := range fn.Phis {
			b := fn.Blocks[p.Block]
			slot := p.Slot.Index
			if !p.Slot.Local {
				slot += len(b.In.Locals)
			}
			dst := ensure(Origin{Kind: OriginPhi, PC: b.First, Slot: slot, Aux: int(b.ID)})
			for _, op := range p.Operands {
				for k, v := range ensure(op.Origin) {
					if v && !dst[k] {
						dst[k] = true
						changed = true
					}
				}
			}
		}
		if !changed {
			out := make([][][]bool, n)
			for i, b := range fn.Blocks {
				if b.Reachable {
					out[i] = make([][]bool, slots)
					for j := 0; j < slots; j++ {
						out[i][j] = ensure(b.InOrig[j])
					}
				}
			}
			return out
		}
	}
	t.Fatal("phi expansion did not converge")
	return nil
}

func TestThousandProductionGraphsAgainstIndependentOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20260922))
	for trial := 0; trial < 1000; trial++ {
		n := 2 + rng.Intn(7)
		slots := 1 + rng.Intn(3)
		code := []byte{}
		for i := 0; i < n-1; i++ {
			if rng.Intn(3) == 0 {
				code = append(code, core.OP_NOP)
			} else {
				code = append(code, core.OP_IINC, byte(rng.Intn(slots)), 1)
			}
		}
		code = append(code, core.OP_RETURN)
		desc := "("
		for j := 0; j < slots; j++ {
			desc += "I"
		}
		desc += ")V"
		ir := irOf(t, code, desc, nil)
		if len(ir.Blocks) != n || len(ir.Instrs) != n {
			t.Fatal("oracle requires one instruction per block")
		}
		ir.Edges = nil
		for from := 0; from < n-1; from++ {
			for to := 0; to < n; to++ {
				if rng.Intn(4) == 0 {
					f, d := ir.Instrs[from].ID, ir.Instrs[to].ID
					id := methodir.EdgeID{From: f, To: d, Kind: core.EdgeTaken}
					ir.Edges = append(ir.Edges, methodir.Edge{ID: id, From: f, To: d, Kind: core.EdgeTaken})
				}
			}
		}
		want := definitionSets(ir, slots)
		for _, opt := range []Options{{}, {LIFO: true}, {Shuffle: true, ShuffleSeed: int64(trial)}} {
			fn, err := Build(ir, opt)
			if err != nil {
				t.Fatalf("trial %d options %+v code %v edges %v: %v", trial, opt, code, ir.Edges, err)
			}
			got := expandedSSA(t, fn, slots)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("trial %d options %+v code %v edges %v\ngot %v\nwant %v\n%s", trial, opt, code, ir.Edges, got, want, fn.Normalize())
			}
		}
	}
}

func TestExceptionContributionsUseBeforeLocals(t *testing.T) {
	code := []byte{core.OP_ICONST_1, core.OP_ISTORE_0, core.OP_INVOKESTATIC, 0, 1, core.OP_ICONST_2, core.OP_ISTORE_0, core.OP_RETURN, core.OP_ASTORE_1, core.OP_RETURN}
	ir := irOf(t, code, "()V", []*core.ExceptionTableEntry{{StartPc: 2, EndPc: 5, HandlerPc: 8}})
	for i := range ir.Instrs {
		if ir.Instrs[i].Opcode == core.OP_INVOKESTATIC {
			ir.Instrs[i].Desc = "()V"
		}
	}
	fn := ssaOf(t, ir)
	found := false
	for _, e := range ir.Edges {
		if e.Kind == core.EdgeException {
			found = true
			st := fn.EdgeStates[e.ID]
			if st.Origins[0] != (Origin{Kind: OriginInstr, PC: 0}) {
				t.Fatalf("postthrow store leaked: %+v", st.Origins)
			}
		}
	}
	if !found {
		t.Fatal("fixture contains no exception edge")
	}
	for _, r := range fn.Instructions {
		if r.PC == 2 && (r.BeforeOrigins[0].PC != 0 || r.Before.Locals[0].Kind != frametransfer.Int) {
			t.Fatal(fmt.Sprint("missing prethrow snapshot: ", r))
		}
	}
}

func TestConstructorExceptionDoesNotPublishInitializedAlias(t *testing.T) {
	code := []byte{core.OP_NEW, 0, 1, core.OP_DUP, core.OP_ASTORE_0, core.OP_INVOKESPECIAL, 0, 2, core.OP_RETURN, core.OP_ASTORE_1, core.OP_RETURN}
	ir := irOf(t, code, "()V", []*core.ExceptionTableEntry{{StartPc: 5, EndPc: 8, HandlerPc: 9}})
	for i := range ir.Instrs {
		ins := &ir.Instrs[i]
		if ins.Opcode == core.OP_NEW {
			ins.Class = "T"
		}
		if ins.Opcode == core.OP_INVOKESPECIAL {
			ins.Class = "T"
			ins.Member = "<init>"
			ins.Desc = "()V"
		}
	}
	fn := ssaOf(t, ir)
	haveException, haveNormal := false, false
	for _, e := range ir.Edges {
		if e.From != 5 {
			continue
		}
		st := fn.EdgeStates[e.ID]
		if e.Kind == core.EdgeException {
			haveException = true
			if st.Frame.Locals[0].Kind != frametransfer.Top || st.Origins[0].Kind != OriginTop {
				t.Fatalf("constructor failure published alias: %+v", st)
			}
		} else {
			haveNormal = true
			if st.Frame.Locals[0].Kind != frametransfer.Ref || st.Origins[0] != (Origin{Kind: OriginInstr, PC: 0}) {
				t.Fatalf("constructor changed object identity: %+v", st)
			}
		}
	}
	if !haveException || !haveNormal {
		t.Fatal("fixture lacks constructor successors")
	}
}

func TestValueBindingsUniqueAndRefreshed(t *testing.T) {
	ir := irOf(t, []byte{core.OP_ILOAD_0, core.OP_INEG, core.OP_ISTORE_0, core.OP_GOTO, 0xff, 0xfd}, "(I)V", nil)
	fn := ssaOf(t, ir)
	byOrigin := map[Origin]ValueID{}
	byID := map[ValueID]bool{}
	for _, v := range fn.Values {
		if v.ID == 0 || byID[v.ID] {
			t.Fatalf("duplicate/zero value ID %d", v.ID)
		}
		byID[v.ID] = true
		byOrigin[v.Origin] = v.ID
	}
	if len(fn.Params) != 1 || fn.Params[0] != byOrigin[Origin{Kind: OriginParam, Slot: 0}] {
		t.Fatal("missing bound parameter")
	}
	for _, r := range fn.Instructions {
		if len(r.UseIDs) != len(r.Uses) || len(r.ResultIDs) != len(r.Results) {
			t.Fatal("missing instruction bindings")
		}
		for i, o := range r.Uses {
			if r.UseIDs[i] == 0 || r.UseIDs[i] != byOrigin[o] {
				t.Fatal("stale instruction binding")
			}
		}
	}
	for _, p := range fn.Phis {
		for _, o := range p.Operands {
			if o.Val == 0 || o.Val != byOrigin[o.Origin] {
				t.Fatal("stale phi binding")
			}
		}
	}
	for _, opt := range []Options{{LIFO: true}, {Shuffle: true, ShuffleSeed: 17}} {
		other, err := Build(ir, opt)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(fn.Values, other.Values) {
			t.Fatal("schedule changed value IDs")
		}
	}
}
