package ssalower

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

func TestDraftFollowupOriginIdentityDoesNotCollide(t *testing.T) {
	a := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: 10, Slot: 100}
	b := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: 11, Slot: 0}
	c := a
	c.Aux = 1
	origins := []ssabuild.Origin{a, b, c, {Kind: ssabuild.OriginInstr, PC: 10, Slot: -1}, {Kind: ssabuild.OriginInstr, PC: 65535, Slot: 65535, Aux: 65535}}
	keys := []ValueKey{}
	for _, o := range origins {
		keys = append(keys, OriginKey(o))
	}
	r, err := NewValueRegistry(keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[VarID]bool{}
	for _, o := range origins {
		id, err := r.Origin(o)
		if err != nil || id == 0 || seen[id] {
			t.Fatalf("collision: %+v id=%d err=%v", o, id, err)
		}
		seen[id] = true
	}
	if _, err := r.Origin(ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 123}); err == nil {
		t.Fatal("missing key accepted")
	}
}
func TestRegistryAliasesAndDeterminism(t *testing.T) {
	use := ValueKey{Kind: int(ssabuild.OriginPhi), PC: 42, Slot: 101, Aux: 8}
	def := ValueKey{Domain: 1, Aux: 27}
	ordinary := ValueKey{Kind: int(ssabuild.OriginInstr), PC: 42, Slot: 101, Aux: 8}
	keys := []ValueKey{ordinary, use, def}
	a, err := NewValueRegistry(keys, map[ValueKey]ValueKey{use: def})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewValueRegistry([]ValueKey{def, use, ordinary}, map[ValueKey]ValueKey{use: def})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		x, _ := a.Lookup(k)
		y, _ := b.Lookup(k)
		if x != y {
			t.Fatal("order changed identity")
		}
	}
	x, _ := a.Lookup(use)
	y, _ := a.Lookup(def)
	z, _ := a.Lookup(ordinary)
	if x != y || x == z {
		t.Fatal("phi use/def not aliased exclusively")
	}
	if _, err := NewValueRegistry(nil, map[ValueKey]ValueKey{use: def, def: use}); err == nil {
		t.Fatal("alias cycle accepted")
	}
	a.next = VarID(int(^uint(0) >> 1))
	if _, err := a.Fresh(); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestCheckedParallelCopies5000Cases(t *testing.T) {
	rng := rand.New(rand.NewSource(2026092204))
	for trial := 0; trial < 5000; trial++ {
		n := 1 + rng.Intn(40)
		copies := make([]Copy, n)
		initial := map[VarID]int64{}
		for i := 1; i <= n+5; i++ {
			initial[VarID(i)] = rng.Int63()
		}
		for i := range copies {
			copies[i] = Copy{Dst: VarID(i + 1), Src: VarID(1 + rng.Intn(n+5))}
		}
		schedule := func(cs []Copy) []Move {
			next := VarID(n + 5)
			moves, err := SequentializeChecked(cs, func() VarID { next++; return next })
			if err != nil {
				t.Fatalf("trial=%d copies=%v err=%v", trial, cs, err)
			}
			return moves
		}
		moves := schedule(copies)
		reversed := append([]Copy(nil), copies...)
		for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
			reversed[i], reversed[j] = reversed[j], reversed[i]
		}
		if other := schedule(reversed); !reflect.DeepEqual(moves, other) {
			t.Fatalf("trial=%d nondeterministic: %v vs %v", trial, moves, other)
		}
		// Independent oracle snapshots sources before ANY assignment.
		want := map[VarID]int64{}
		got := map[VarID]int64{}
		for id, v := range initial {
			want[id] = v
			got[id] = v
		}
		for _, c := range copies {
			want[c.Dst] = initial[c.Src]
		}
		fresh := map[VarID]bool{}
		for _, m := range moves {
			if m.Tmp {
				if _, live := initial[m.Dst]; live || fresh[m.Dst] {
					t.Fatalf("trial=%d nonfresh temp %d", trial, m.Dst)
				}
				fresh[m.Dst] = true
			}
			v, ok := got[m.Src]
			if !ok {
				t.Fatalf("trial=%d undefined source %d", trial, m.Src)
			}
			got[m.Dst] = v
		}
		for id, v := range want {
			if got[id] != v {
				t.Fatalf("trial=%d copies=%v moves=%v id=%d got=%d want=%d", trial, copies, moves, id, got[id], v)
			}
		}
	}
}
func TestCheckedCopiesRejectInvalidInputs(t *testing.T) {
	cases := [][]Copy{{{1, 1}, {1, 2}}, {{1, 2}, {1, 3}}, {{0, 1}}, {{1, 0}}, {{-1, 2}}}
	for _, cs := range cases {
		if moves, err := SequentializeChecked(cs, nil); err == nil || moves != nil {
			t.Fatalf("accepted %v", cs)
		}
	}
	for _, bad := range []VarID{0, -1, 1, 2} {
		if moves, err := SequentializeChecked([]Copy{{1, 2}, {2, 1}}, func() VarID { return bad }); err == nil || moves != nil {
			t.Fatalf("accepted temp %d", bad)
		}
	}
	// A repeated allocator value is invalid even after the first cycle finished.
	if moves, err := SequentializeChecked([]Copy{{1, 2}, {2, 1}, {3, 4}, {4, 3}}, func() VarID { return 8 }); err == nil || moves != nil {
		t.Fatal("reused temp accepted")
	}
}

func TestLowerWorkCounterIsTransactionalAndPreservesPlan(t *testing.T) {
	code := []byte{
		core.OP_ILOAD_0, core.OP_IFEQ, 0, 7,
		core.OP_ICONST_1, core.OP_GOTO, 0, 4,
		core.OP_ICONST_2,
		core.OP_IRETURN,
	}
	fn, want := ssaDestroy(t, code, "(I)I", nil)
	before := fn.Normalize()

	complete := &ssabuild.LimitCounter{Max: 100_000}
	got, err := DestroyWithWorkCounter(fn, Options{MaxSpills: 1000}, complete)
	if err != nil || got == nil {
		t.Fatalf("sufficient work budget rejected lowering: plan=%+v err=%v used=%d", got, err, complete.Used)
	}
	if complete.Used == 0 {
		t.Fatal("successful lowering did not report its work")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("work accounting changed the emission plan\n got: %+v\nwant: %+v", got, want)
	}
	if complete.Used < 2 {
		t.Fatalf("fixture did not traverse enough stages to test late exhaustion: used=%d", complete.Used)
	}

	// Exhaust on the final unit of the successful run so the method has already
	// built most of the private plan; callers must still receive nil, not that
	// partly validated plan.
	exhausted := &ssabuild.LimitCounter{Max: complete.Used - 1}
	partial, err := DestroyWithWorkCounter(fn, Options{MaxSpills: 1000}, exhausted)
	if err == nil || partial != nil {
		t.Fatalf("budget exhaustion returned partial lowering plan: plan=%+v err=%v", partial, err)
	}
	if exhausted.Used > exhausted.Max {
		t.Fatalf("counter exceeded its limit: used=%d max=%d", exhausted.Used, exhausted.Max)
	}
	if got := fn.Normalize(); got != before {
		t.Fatal("budgeted lowering mutated its SSA input")
	}
}

func TestRegistryTemporaryPreservesLogicalMetadata(t *testing.T) {
	keys := []ValueKey{{Slot: 1}, {Slot: 2}, {Slot: 99999}}
	r, err := NewValueRegistry(keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := r.Lookup(keys[0])
	b, _ := r.Lookup(keys[1])
	unrelated, _ := r.Lookup(keys[2])
	origin := ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}
	for _, id := range []VarID{a, b} {
		r.Info[id] = ValueInfo{Type: frametransfer.T(frametransfer.Long), Width: 2, Origin: origin}
	}
	moves, err := r.sequentialize([]Copy{{a, b}, {b, a}})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range moves {
		if m.Tmp {
			info := r.Info[m.Dst]
			if m.Dst <= unrelated || info.Width != 2 || info.Type.Kind != frametransfer.Long || info.Origin != origin || !info.Temporary || info.Source != m.Src {
				t.Fatalf("temporary lost identity/type: %+v %+v", m, info)
			}
		}
	}
}

func TestPhiDefinitionUseAndEntryCopies(t *testing.T) {
	code := []byte{core.OP_ILOAD_0, core.OP_INEG, core.OP_ISTORE_0, core.OP_GOTO, 0xff, 0xfd}
	fn, low := ssaDestroy(t, code, "(I)V", nil)
	if len(low.EntryMoves) != 1 {
		t.Fatalf("entry phi not initialized: %v", low.EntryMoves)
	}
	entry := low.EntryMoves[0]
	if entry.Src == entry.Dst {
		t.Fatal("entry value and phi definition collapsed")
	}
	found := false
	for _, p := range fn.Phis {
		if p.Block == fn.Blocks[0].ID && p.Slot.Local && p.Slot.Index == 0 {
			dest, err := low.Values.Phi(p)
			if err != nil {
				t.Fatal(err)
			}
			if dest != entry.Dst {
				t.Fatal("entry assigned unrelated variable")
			}
			for _, v := range fn.Instructions {
				if v.PC == 1 {
					if len(v.Uses) != 1 {
						t.Fatal("fixture lost negation use")
					}
					use, err := low.Values.Origin(v.Uses[0])
					if err != nil {
						t.Fatal(err)
					}
					if use != dest {
						t.Fatalf("phi def=%d use=%d", dest, use)
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("entry phi use not checked")
	}
}
