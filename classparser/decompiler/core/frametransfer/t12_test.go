package frametransfer

import (
	"math"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

func TestT12(t *testing.T) {
	t.Run("T12-C01", TestT12_C01_DupFamily)
	t.Run("T12-C02", TestT12_C02_Category2Overlap)
	t.Run("T12-C03", TestT12_C03_InitAliases)
	t.Run("T12-C04", TestT12_C04_StackMapTable)
	t.Run("T12-C05", TestT12_C05_NumericEdges)
	t.Run("T12-C06", TestT12_C06_TransferFuzz)
}

func mustTransfer(t *testing.T, in Frame, op int) Frame {
	t.Helper()
	out, _, err := Transfer(in, Instr{Op: op, Local: -1})
	if err != nil {
		t.Fatalf("op %#x: %v (in %s)", op, err, in.Canonical())
	}
	return out
}

func mustFail(t *testing.T, in Frame, op int) {
	t.Helper()
	_, _, err := Transfer(in, Instr{Op: op, Local: -1})
	if err == nil || !IsInvalid(err) {
		t.Fatalf("expected invalid op %#x in %s, got %v", op, in.Canonical(), err)
	}
}

func stack(kinds ...Type) Frame {
	f := NewFrame(4)
	for _, k := range kinds {
		if err := f.push(k); err != nil {
			panic(err)
		}
	}
	return f
}

func TestT12_C01_DupFamily(t *testing.T) {
	I, F, R := T(Int), T(Float), RefOf("java/lang/Object")
	J := T(Long)
	D := T(Double)
	legal := []struct {
		op   int
		in   []Type
		want []Type
	}{
		{core.OP_DUP, []Type{I}, []Type{I, I}},
		{core.OP_DUP, []Type{R}, []Type{R, R}},
		{core.OP_DUP, []Type{F}, []Type{F, F}},
		{core.OP_DUP_X1, []Type{I, F}, []Type{F, I, F}},
		{core.OP_DUP_X1, []Type{R, I}, []Type{I, R, I}},
		{core.OP_DUP_X2, []Type{I, F, R}, []Type{R, I, F, R}},
		{core.OP_DUP_X2, []Type{J, I}, []Type{I, J, I}},
		{core.OP_DUP2, []Type{I, F}, []Type{I, F, I, F}},
		{core.OP_DUP2, []Type{J}, []Type{J, J}},
		{core.OP_DUP2, []Type{D}, []Type{D, D}},
		{core.OP_DUP2_X1, []Type{R, I, F}, []Type{I, F, R, I, F}},
		{core.OP_DUP2_X1, []Type{I, J}, []Type{J, I, J}},
		{core.OP_DUP2_X2, []Type{I, F, R, T(Int)}, []Type{R, T(Int), I, F, R, T(Int)}},
		{core.OP_DUP2_X2, []Type{I, F, J}, []Type{J, I, F, J}},
		{core.OP_DUP2_X2, []Type{J, I, F}, []Type{I, F, J, I, F}},
		{core.OP_DUP2_X2, []Type{J, D}, []Type{D, J, D}},
		{core.OP_SWAP, []Type{I, F}, []Type{F, I}},
		{core.OP_POP, []Type{I}, []Type{}},
		{core.OP_POP2, []Type{I, F}, []Type{}},
		{core.OP_POP2, []Type{J}, []Type{}},
	}
	for _, tc := range legal {
		in := stack(tc.in...)
		out := mustTransfer(t, in, tc.op)
		want := stack(tc.want...)
		if len(out.Stack) != len(want.Stack) {
			t.Fatalf("%#x stack %s want %s", tc.op, out.Canonical(), want.Canonical())
		}
		for i := range out.Stack {
			if out.Stack[i].Kind != want.Stack[i].Kind {
				t.Fatalf("%#x stack %s want %s", tc.op, out.Canonical(), want.Canonical())
			}
		}
	}
	illegal := []struct {
		op int
		in []Type
	}{
		{core.OP_DUP, []Type{J}},
		{core.OP_DUP, []Type{D}},
		{core.OP_DUP, nil},
		{core.OP_DUP_X1, []Type{I}},
		{core.OP_DUP_X1, []Type{J, I}},
		{core.OP_DUP_X1, []Type{I, J}},
		{core.OP_DUP_X2, []Type{I}},
		{core.OP_DUP2, []Type{I}},
		{core.OP_DUP2_X1, []Type{J}},
		{core.OP_SWAP, []Type{J, I}},
		{core.OP_SWAP, []Type{I, J}},
		{core.OP_POP, []Type{J}},
	}
	for _, tc := range illegal {
		mustFail(t, stack(tc.in...), tc.op)
	}
}

func TestT12_C02_Category2Overlap(t *testing.T) {
	f := NewFrame(4)
	if err := f.StoreLocal(0, LongConst(9)); err != nil {
		t.Fatal(err)
	}
	if f.Locals[0].Kind != Long || f.Locals[1].Kind != LongTail {
		t.Fatalf("pair %s", f.Canonical())
	}
	in := f.Clone()
	if err := in.push(T(Int)); err != nil {
		t.Fatal(err)
	}
	out, _, err := Transfer(in, Instr{Op: core.OP_ISTORE_1, Local: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Locals[0].Kind != Top {
		t.Fatalf("head not invalidated: %s", out.Canonical())
	}
	if out.Locals[1].Kind != Int {
		t.Fatalf("tail store %s", out.Canonical())
	}
	if out.Locals[0].Kind == Null || out.Locals[1].Kind == Null || out.Locals[1].Kind == Long || out.Locals[1].Kind == LongTail {
		t.Fatalf("tail kept old/null: %s", out.Canonical())
	}
	in2 := f.Clone()
	if err := in2.push(T(Int)); err != nil {
		t.Fatal(err)
	}
	out2, _, err := Transfer(in2, Instr{Op: core.OP_ISTORE_0, Local: 0})
	if err != nil {
		t.Fatal(err)
	}
	if out2.Locals[0].Kind != Int || out2.Locals[1].Kind != Top {
		t.Fatalf("head store %s", out2.Canonical())
	}
	in3 := f.Clone()
	out3, _, err := Transfer(in3, Instr{Op: core.OP_IINC, Local: 1, IincConst: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out3.Locals[0].Kind == Long || out3.Locals[1].Kind == LongTail || out3.Locals[1].Kind == Null {
		t.Fatalf("iinc overlap %s", out3.Canonical())
	}
}

func TestT12_C03_InitAliases(t *testing.T) {
	f := NewFrame(4)
	out, _, err := Transfer(f, Instr{Op: core.OP_NEW, PC: 10, Class: "Foo"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Stack[0].Kind != UninitNew || out.Stack[0].NewPC != 10 {
		t.Fatalf("new: %s", out.Canonical())
	}
	duped := mustTransfer(t, out, core.OP_DUP)
	if duped.Stack[0].NewPC != 10 || duped.Stack[1].NewPC != 10 {
		t.Fatalf("dup alias %s", duped.Canonical())
	}
	st := duped.Clone()
	if err := st.StoreLocal(1, st.Stack[0]); err != nil {
		t.Fatal(err)
	}
	_, _ = st.popValue()
	initIn := st
	outInit, ex, err := Transfer(initIn, Instr{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "Foo", Desc: "()V"})
	if err != nil {
		t.Fatal(err)
	}
	if outInit.Locals[1].Kind != Ref {
		t.Fatalf("successful init %s", outInit.Canonical())
	}
	if ex == nil || ex.Locals[1].Kind != UninitNew || ex.Locals[1].NewPC != 10 {
		t.Fatalf("exception must keep uninit: %+v", ex)
	}
	f2 := NewFrame(4)
	a, _, _ := Transfer(f2, Instr{Op: core.OP_NEW, PC: 10, Class: "Foo"})
	_ = a.StoreLocal(1, a.Stack[0])
	_, _ = a.popValue()
	b, _, _ := Transfer(a, Instr{Op: core.OP_NEW, PC: 20, Class: "Foo"})
	_ = b.StoreLocal(2, b.Stack[0])
	_, _ = b.popValue()
	_ = b.push(b.Locals[1])
	done, _, err := Transfer(b, Instr{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "Foo", Desc: "()V"})
	if err != nil {
		t.Fatal(err)
	}
	if done.Locals[1].Kind != Ref {
		t.Fatalf("site 10 should init: %s", done.Canonical())
	}
	if done.Locals[2].Kind != UninitNew || done.Locals[2].NewPC != 20 {
		t.Fatalf("distinct new sites merged: %s", done.Canonical())
	}
	ctor := NewFrame(2)
	_ = ctor.StoreLocal(0, T(UninitThis))
	_ = ctor.push(ctor.Locals[0])
	after, ex2, err := Transfer(ctor, Instr{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "java/lang/Object", Desc: "()V"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Locals[0].Kind != Ref {
		t.Fatalf("this init %s", after.Canonical())
	}
	if ex2 == nil || ex2.Locals[0].Kind != UninitThis {
		t.Fatalf("super init exception %v", ex2)
	}
}

func TestT12_C04_StackMapTable(t *testing.T) {
	f := NewFrame(1)
	_ = f.StoreLocal(0, T(Int))
	computed := map[uint16]Frame{0: f, 4: f}
	if err := CheckStackMapTable(49, nil, computed, nil); err != nil {
		t.Fatalf("v49 without SMT rejected: %v", err)
	}
	smt := EncodeFullFrame(0, f.Locals, nil)
	if err := CheckStackMapTable(52, smt, computed, nil); err != nil {
		t.Fatalf("consistent SMT: %v", err)
	}
	bad := NewFrame(1)
	_ = bad.StoreLocal(0, T(Float))
	if err := CheckStackMapTable(52, EncodeFullFrame(0, bad.Locals, nil), computed, nil); err == nil {
		t.Fatal("inconsistent SMT accepted")
	}
}

func TestT12_C05_NumericEdges(t *testing.T) {
	nan := math.Float64bits(math.NaN())
	neg0 := math.Float64bits(math.Copysign(0, -1))
	pos0 := math.Float64bits(0)
	f := NewFrame(8)
	in := f.Clone()
	if err := in.push(DoubleBits(nan)); err != nil {
		t.Fatal(err)
	}
	st, _, err := Transfer(in, Instr{Op: core.OP_DSTORE, Local: 2})
	if err != nil {
		t.Fatal(err)
	}
	ld, _, err := Transfer(st, Instr{Op: core.OP_DLOAD, Local: 2})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ld.popValue()
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasBits || got.Bits != nan {
		t.Fatalf("NaN bits lost %#x vs %#x", got.Bits, nan)
	}
	in2 := f.Clone()
	_ = in2.push(DoubleBits(neg0))
	st2, _, _ := Transfer(in2, Instr{Op: core.OP_DSTORE, Local: 4})
	ld2, _, _ := Transfer(st2, Instr{Op: core.OP_DLOAD, Local: 4})
	g2, _ := ld2.popValue()
	if !g2.HasBits || g2.Bits != neg0 || g2.Bits == pos0 {
		t.Fatalf("-0.0 identity lost %#x", g2.Bits)
	}
	add := stack(IntConst(math.MaxInt32), IntConst(1))
	out := mustTransfer(t, add, core.OP_IADD)
	if out.Stack[0].Kind != Int {
		t.Fatalf("iadd width %s", out.Canonical())
	}
	wide := NewFrame(302)
	_ = wide.push(IntConst(5))
	stw, _, err := Transfer(wide, Instr{Op: core.OP_ISTORE, Local: 300, Wide: true})
	if err != nil {
		t.Fatal(err)
	}
	ldw, _, err := Transfer(stw, Instr{Op: core.OP_ILOAD, Local: 300, Wide: true})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := ldw.popValue()
	if v.Kind != Int || !v.HasInt || v.Int != 5 {
		t.Fatalf("wide slot 300 %s", ldw.Canonical())
	}
	sh := stack(IntConst(-1), IntConst(33))
	out = mustTransfer(t, sh, core.OP_IUSHR)
	if out.Stack[0].Kind != Int {
		t.Fatalf("iushr %s", out.Canonical())
	}
	_ = methodir.SnapshotVersion
}

func TestT12_C06_TransferFuzz(t *testing.T) {
	ops := []int{
		core.OP_NOP, core.OP_POP, core.OP_POP2, core.OP_DUP, core.OP_DUP_X1, core.OP_DUP_X2,
		core.OP_DUP2, core.OP_DUP2_X1, core.OP_DUP2_X2, core.OP_SWAP,
		core.OP_IADD, core.OP_ISUB, core.OP_ILOAD, core.OP_ISTORE, core.OP_IINC,
		core.OP_LADD, core.OP_LLOAD, core.OP_LSTORE, core.OP_DADD, core.OP_DLOAD, core.OP_DSTORE,
		core.OP_ACONST_NULL, core.OP_ICONST_0, core.OP_LCONST_0, core.OP_DCONST_0,
		core.OP_GOTO, core.OP_IFEQ, core.OP_IRETURN,
	}
	kinds := []Type{T(Int), T(Float), T(Long), T(Double), RefOf("java/lang/Object"), T(Null)}
	rng := newFuzzRNG(0xC0FFEE)
	for i := 0; i < 4000; i++ {
		f := NewFrame(8)
		n := rng.Intn(4)
		ok := true
		for j := 0; j < n; j++ {
			k := kinds[rng.Intn(len(kinds))]
			if err := f.push(k); err != nil {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		op := ops[rng.Intn(len(ops))]
		in := Instr{Op: op, Local: rng.Intn(6), IincConst: int32(rng.Intn(5) - 2), PC: uint16(i)}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic op %#x in %s: %v", op, f.Canonical(), r)
				}
			}()
			out, _, err := Transfer(f, in)
			if err == nil {
				if len(out.Stack) > maxSlots || len(out.Locals) > maxSlots {
					t.Fatalf("unbounded alloc %d %d", len(out.Stack), len(out.Locals))
				}
			} else if !IsInvalid(err) && !IsUnsupported(err) {
				t.Fatalf("unexpected %v", err)
			}
		}()
	}
}

type fuzzRNG struct{ s uint64 }

func newFuzzRNG(seed uint64) *fuzzRNG { return &fuzzRNG{s: seed} }

func (r *fuzzRNG) Intn(n int) int {
	r.s = r.s*6364136223846793005 + 1
	if n <= 0 {
		return 0
	}
	return int((r.s >> 33) % uint64(n))
}

func TestT12SupportedOpcodeTableNoPanic(t *testing.T) {
	t.Run("T12-C06", func(t *testing.T) {
		skip := map[int]bool{core.OP_WIDE: true, core.OP_JSR: true, core.OP_JSR_W: true, core.OP_RET: true}
		f := NewFrame(8)
		_ = f.push(T(Int))
		for op := 0; op < 256; op++ {
			if skip[op] {
				_, _, err := Transfer(f, Instr{Op: op, Local: 0})
				if err == nil {
					t.Fatalf("jsr/wide/ret %#x should be unsupported or invalid", op)
				}
				continue
			}
			func(op int) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("opcode %#x panicked: %v", op, r)
					}
				}()
				_, _, _ = Transfer(f, Instr{Op: op, Local: 0, Class: "java/lang/Object", Desc: "()V"})
			}(op)
		}
	})
}
