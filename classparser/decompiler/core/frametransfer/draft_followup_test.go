// Copy to classparser/decompiler/core/frametransfer/draft_followup_test.go.
// These are intentionally strict regression tests. Expected to expose the pinned draft.
package frametransfer

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"testing"
)

func TestDraftFollowupALoadRejectsPrimitive(t *testing.T) {
	for _, k := range []Kind{Int, Float} {
		f := NewFrame(1)
		f.Locals[0] = T(k)
		if _, err := f.loadLocal(0, Ref); err == nil {
			t.Fatalf("aload accepted %s", k)
		}
	}
}
func TestDraftFollowupCategory2TailMustMatch(t *testing.T) {
	f := NewFrame(2)
	f.Locals[0] = T(Long)
	f.Locals[1] = T(DoubleTail)
	if _, err := f.loadLocal(0, Long); err == nil {
		t.Fatal("long load accepted double tail")
	}
}

func TestDraftFollowupLoadsStoresAndBounds(t *testing.T) {
	values := []Type{T(Top), T(Int), T(Float), T(Long), T(Double), T(LongTail), T(DoubleTail), RefOf("C"), T(Null), T(UninitThis), UninitAt(7)}
	for _, value := range values {
		for _, want := range []Kind{Int, Float, Long, Double, Ref} {
			f := NewFrame(2)
			f.Locals[0] = value
			if value.Kind == Long {
				f.Locals[1] = T(LongTail)
			}
			if value.Kind == Double {
				f.Locals[1] = T(DoubleTail)
			}
			_, err := f.loadLocal(0, want)
			valid := value.Kind == want || want == Ref && (value.Kind == Null || value.Kind == UninitThis || value.Kind == UninitNew)
			if (err == nil) != valid {
				t.Fatalf("load %v as %v: %v", value, want, err)
			}
		}
	}
	f := NewFrame(2)
	for _, idx := range []int{-1, 2, 65535, int(^uint(0) >> 1)} {
		before := f.Clone()
		if err := f.StoreLocal(idx, T(Long)); err == nil {
			t.Fatalf("accepted store %d", idx)
		}
		if _, err := f.loadLocal(idx, Int); err == nil {
			t.Fatalf("accepted load %d", idx)
		}
		if !f.Equal(before) {
			t.Fatal("invalid access mutated frame")
		}
	}
	if err := f.StoreLocal(1, T(Long)); err == nil {
		t.Fatal("wide store exceeds max_locals")
	}
	for _, value := range []Type{T(Top), T(LongTail), T(DoubleTail), T(Kind(255))} {
		if err := f.StoreLocal(0, value); err == nil {
			t.Fatalf("stored invalid %v", value)
		}
		if err := f.push(value); err == nil {
			t.Fatalf("pushed invalid %v", value)
		}
	}
}

func TestDraftFollowupDeclaredStackLimit(t *testing.T) {
	for _, limits := range [][2]int{{-1, 1}, {65536, 1}, {1, -1}, {1, 65536}} {
		if _, err := NewFrameWithLimits(limits[0], limits[1]); err == nil {
			t.Fatal("accepted bad limits", limits)
		}
	}
	f, err := NewFrameWithLimits(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Transfer(f, Instr{Op: core.OP_LCONST_0})
	if err != nil || len(out.Stack) != 2 {
		t.Fatalf("wide push %v %v", out, err)
	}
	if _, _, err := Transfer(out, Instr{Op: core.OP_ICONST_0}); err == nil {
		t.Fatal("max_stack ignored")
	}
	if len(f.Stack) != 0 {
		t.Fatal("transfer mutated input")
	}
	zero, _ := NewFrameWithLimits(0, 0)
	if _, _, err := Transfer(zero, Instr{Op: core.OP_ICONST_0}); err == nil {
		t.Fatal("zero stack limit ignored")
	}
	_, ex, err := Transfer(zero, Instr{Op: core.OP_INVOKESTATIC, Class: "C", Name: "m", Desc: "()V"})
	if err != nil || ex == nil {
		t.Fatalf("no-handler throwing call with zero stack: %v", err)
	}
	if err := ex.Validate(); err == nil {
		t.Fatal("zero stack method cannot attach an exception handler")
	}
}

func TestDraftFollowupMergeLocalsAndStack(t *testing.T) {
	a, b := NewFrame(4), NewFrame(4)
	a.StoreLocal(0, T(Long))
	a.StoreLocal(2, T(Double))
	b.StoreLocal(0, T(Int))
	b.StoreLocal(1, T(Int))
	b.StoreLocal(2, T(Long))
	joined, err := JoinFrames(a, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range joined.Locals {
		if v.Kind != Top {
			t.Fatal("incompatible dead local remains usable", joined)
		}
	}
	if _, err := joined.loadLocal(0, Int); err == nil {
		t.Fatal("read merged dead local")
	}
	for _, stacks := range [][2][]Type{
		{{T(Int)}, {T(Float)}}, {{T(Long), T(LongTail)}, {T(Int), T(Int)}},
		{{T(Top)}, {T(Int)}}, {{UninitAt(1)}, {UninitAt(2)}},
		{{T(Long), T(DoubleTail)}, {T(Long), T(LongTail)}},
	} {
		x, y := NewFrame(0), NewFrame(0)
		x.Stack, y.Stack = stacks[0], stacks[1]
		if _, err := JoinFrames(x, y); err == nil {
			t.Fatalf("merged incompatible stack %v", stacks)
		}
	}
	a.Stack, b.Stack = []Type{T(Null)}, []Type{RefOf("C")}
	joined, err = JoinFrames(a, b)
	if err != nil || joined.Stack[0] != RefOf("C") {
		t.Fatalf("null/reference merge: %v %v", joined, err)
	}
}

func TestDraftFollowupRandomPairsAndJoins(t *testing.T) {
	rng := newFuzzRNG(0x52F02)
	values := []Type{T(Int), T(Float), T(Long), T(Double), RefOf("C"), T(Null)}
	a, b := NewFrame(12), NewFrame(12)
	check := func(f Frame) {
		t.Helper()
		for i, v := range f.Locals {
			switch v.Kind {
			case Long, Double:
				want := LongTail
				if v.Kind == Double {
					want = DoubleTail
				}
				if i+1 == len(f.Locals) || f.Locals[i+1].Kind != want {
					t.Fatalf("orphan head at %d: %v", i, f)
				}
			case LongTail, DoubleTail:
				want := Long
				if v.Kind == DoubleTail {
					want = Double
				}
				if i == 0 || f.Locals[i-1].Kind != want {
					t.Fatalf("orphan tail at %d: %v", i, f)
				}
			}
		}
	}
	for n := 0; n < 2000; n++ {
		for _, f := range []*Frame{&a, &b} {
			v := values[rng.Intn(len(values))]
			idx := rng.Intn(12)
			before := f.Clone()
			err := f.StoreLocal(idx, v)
			if err != nil && !f.Equal(before) {
				t.Fatalf("failed store mutated frame: iteration %d", n)
			}
			check(*f)
		}
		aBefore, bBefore := a.Clone(), b.Clone()
		ab, err := JoinFrames(a, b)
		if err != nil {
			t.Fatalf("iteration %d: %v", n, err)
		}
		check(ab)
		ba, err := JoinFrames(b, a)
		if err != nil || !ab.Equal(ba) {
			t.Fatalf("noncommutative join iteration %d", n)
		}
		aa, err := JoinFrames(a, a)
		if err != nil || !aa.Equal(a) {
			t.Fatalf("nonidempotent join iteration %d", n)
		}
		if !a.Equal(aBefore) || !b.Equal(bBefore) {
			t.Fatal("join mutated input")
		}
	}
}

func TestDraftFollowupConstructorPaths(t *testing.T) {
	f := NewFrame(3)
	f.ThisClass, f.DirectSuperClass = "Child", "Parent"
	if err := f.StoreLocal(0, T(UninitThis)); err != nil {
		t.Fatal(err)
	}
	f.StoreLocal(1, T(UninitThis))
	other := UninitAt(99)
	other.Class = "Other"
	f.StoreLocal(2, other)
	f.Stack = []Type{T(UninitThis), T(UninitThis), T(Int)}
	before := f.Clone()
	out, ex, err := Transfer(f, Instr{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "Parent", Desc: "(I)V"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ThisUninitialized || out.Locals[0] != RefOf("Child") || out.Locals[1] != RefOf("Child") || len(out.Stack) != 1 || out.Stack[0] != RefOf("Child") {
		t.Fatalf("normal init aliases: %v", out)
	}
	if ex == nil || !ex.ThisUninitialized || ex.Locals[0].Kind != Top || ex.Locals[1].Kind != Top || ex.Locals[2] != other || len(ex.Stack) != 1 || ex.Stack[0] != RefOf("java/lang/Throwable") {
		t.Fatalf("exception aliases: %v", ex)
	}
	if _, _, err := Transfer(*ex, Instr{Op: core.OP_ALOAD, Local: 0}); err == nil {
		t.Fatal("handler reloaded broken receiver")
	}
	if _, _, err := Transfer(*ex, Instr{Op: core.OP_RETURN}); err == nil {
		t.Fatal("handler bypassed constructor init")
	}
	if !f.Equal(before) {
		t.Fatal("constructor mutated input")
	}
	for _, tc := range []Instr{
		{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "Wrong", Desc: "(I)V"},
		{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "Parent", Desc: "(I)I"},
		{Op: core.OP_INVOKEVIRTUAL, Name: "<init>", Class: "Parent", Desc: "(I)V"},
		{Op: core.OP_INVOKEVIRTUAL, Name: "m", Class: "Parent", Desc: "(I)V"},
	} {
		if _, _, err := Transfer(f, tc); err == nil {
			t.Fatalf("accepted invalid constructor invocation: %v", tc)
		}
	}
	for _, recv := range []Type{RefOf("Child"), T(Null)} {
		x := NewFrame(0)
		x.Stack = []Type{recv}
		if _, _, err := Transfer(x, Instr{Op: core.OP_INVOKESPECIAL, Name: "<init>", Class: "Child", Desc: "()V"}); err == nil {
			t.Fatal("constructor accepted initialized/null receiver")
		}
	}
}

func TestDraftFollowupReferenceUseAndIinc(t *testing.T) {
	f := NewFrame(1)
	site := UninitAt(5)
	site.Class = "C"
	f.Stack = []Type{site}
	stored, _, err := Transfer(f, Instr{Op: core.OP_ASTORE, Local: 0})
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := Transfer(stored, Instr{Op: core.OP_ALOAD, Local: 0})
	if err != nil || loaded.Stack[0] != site {
		t.Fatal("uninitialized reference load/store rejected")
	}
	if _, _, err := Transfer(loaded, Instr{Op: core.OP_INVOKEVIRTUAL, Name: "m", Class: "C", Desc: "()V"}); err == nil {
		t.Fatal("ordinary invoke accepted uninitialized reference")
	}
	for _, value := range []Type{T(Top), T(Float), T(Long), T(Double)} {
		x := NewFrame(2)
		if value.Kind != Top {
			x.StoreLocal(0, value)
		}
		if _, _, err := Transfer(x, Instr{Op: core.OP_IINC, Local: 0}); err == nil {
			t.Fatalf("iinc accepted %v", value)
		}
	}
	f.StoreLocal(0, IntConst(41))
	f.Stack = nil
	out, _, err := Transfer(f, Instr{Op: core.OP_IINC, Local: 0, IincConst: 1})
	if err != nil || out.Locals[0] != IntConst(42) {
		t.Fatalf("valid iinc: %v %v", out, err)
	}
}

func TestDraftFollowupDescriptorValidation(t *testing.T) {
	for _, desc := range []string{"", "(", "()", "()Vx", "()II", "(V)V", "([V)V", "(L;)V", "([Q)V"} {
		if _, _, _, err := ParseDescriptor(desc); err == nil {
			t.Fatalf("accepted descriptor %q", desc)
		}
	}
	args, ret, hasRet, err := ParseDescriptor("(J[[DLjava/lang/Object;)I")
	if err != nil || len(args) != 3 || args[0].Kind != Long || args[1] != RefOf("[[D") || args[2] != RefOf("java/lang/Object") || !hasRet || ret.Kind != Int {
		t.Fatalf("valid descriptor: %v %v %v", args, ret, err)
	}
}

func TestDraftFollowupPutfieldBeforeInit(t *testing.T) {
	f := NewFrame(1)
	f.ThisClass = "C"
	f.StoreLocal(0, T(UninitThis))
	f.Stack = []Type{T(UninitThis), T(Int)}
	out, ex, err := Transfer(f, Instr{Op: core.OP_PUTFIELD, Class: "C", Name: "x", Desc: "I"})
	if err != nil || !out.ThisUninitialized || ex == nil || ex.Locals[0].Kind != UninitThis {
		t.Fatalf("current-class pre-init putfield: %v %v", out, err)
	}
	if _, _, err := Transfer(f, Instr{Op: core.OP_PUTFIELD, Class: "Other", Name: "x", Desc: "I"}); err == nil {
		t.Fatal("pre-init putfield accepted unrelated owner")
	}
}
