package ssalower

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

// This contract is computational-category compatibility, not reference-class
// assignability. No class hierarchy is available to the copy scheduler. Distinct
// uninitialized NEW identities must never be converted into an ordinary copy.
func TestTypedCopyRequiresCompatibleComputationalCategories(t *testing.T) {
	types := []frametransfer.Type{frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Float), frametransfer.T(frametransfer.Long), frametransfer.T(frametransfer.Double), frametransfer.RefOf("java/lang/Object"), frametransfer.T(frametransfer.Null), frametransfer.T(frametransfer.UninitThis), frametransfer.UninitAt(10), frametransfer.UninitAt(20), frametransfer.T(frametransfer.Top), frametransfer.T(frametransfer.LongTail)}
	for i, dst := range types {
		for j, src := range types {
			accept := i < 9 && j < 9 && (i == j || i == 4 && j == 5)
			t.Run(fmt.Sprintf("%s_from_%s", dst, src), func(t *testing.T) {
				r, e := NewValueRegistry([]ValueKey{{Slot: 1}, {Slot: 2}}, nil)
				if e != nil {
					t.Fatal(e)
				}
				r.Info[1] = ValueInfo{Type: dst, Width: dst.Width()}
				r.Info[2] = ValueInfo{Type: src, Width: src.Width()}
				before := r.next
				moves, e := r.sequentialize([]Copy{{Dst: 1, Src: 2}})
				if (e == nil) != accept {
					t.Fatalf("category admission=%v want=%v moves=%v error=%v", e == nil, accept, moves, e)
				}
				if !accept && (moves != nil || r.next != before || len(r.Info) != 2) {
					t.Fatal("invalid packet partially published or allocated")
				}
			})
		}
	}
	for _, width := range []int{-1, 0, 1, 3} {
		t.Run(fmt.Sprintf("forged_long_width_%d", width), func(t *testing.T) {
			r, _ := NewValueRegistry([]ValueKey{{Slot: 1}, {Slot: 2}}, nil)
			r.Info[1] = ValueInfo{Type: frametransfer.T(frametransfer.Long), Width: width}
			r.Info[2] = r.Info[1]
			if moves, e := r.sequentialize([]Copy{{1, 2}}); e == nil || moves != nil {
				t.Fatalf("accepted forged type width=%d moves=%v error=%v", width, moves, e)
			}
		})
	}
	t.Run("same NEW PC with conflicting original class", func(t *testing.T) {
		r, _ := NewValueRegistry([]ValueKey{{Slot: 1}, {Slot: 2}}, nil)
		a, b := frametransfer.UninitAt(10), frametransfer.UninitAt(10)
		a.Class, b.Class = "example/First", "example/Second"
		r.Info[1], r.Info[2] = ValueInfo{Type: a, Width: 1}, ValueInfo{Type: b, Width: 1}
		if moves, e := r.sequentialize([]Copy{{1, 2}}); e == nil || moves != nil {
			t.Fatalf("conflicting allocation metadata accepted: moves=%v err=%v", moves, e)
		}
	})
}

// E02 exhausts all 1..4-variable partial maps: a destination is omitted or
// reads any OLD ambient value or one read-only constant (1,440 logical maps).
// All category-1/2 vectors, including the constant, add 43,612 typed scenarios.
// The oracle snapshots old state directly, never ParallelEval/SerialEval.
func TestBoundedPartialParallelCopyModel(t *testing.T) {
	inventory := sha256.New()
	logical, typed, legal, refused := 0, 0, 0, 0
	for n := 1; n <= 4; n++ {
		radix := n + 2
		maps := 1
		for i := 0; i < n; i++ {
			maps *= radix
		}
		for number := 0; number < maps; number++ {
			copies := []Copy{}
			encoded := number
			for dst := 1; dst <= n; dst++ {
				src := encoded % radix
				encoded /= radix
				if src != 0 {
					copies = append(copies, Copy{VarID(dst), VarID(src)})
				}
			}
			logical++
			for widths := 0; widths < 1<<(n+1); widths++ {
				typed++
				keys := []ValueKey{}
				for i := 1; i <= n+2; i++ {
					keys = append(keys, ValueKey{Slot: i})
				}
				r, e := NewValueRegistry(keys, nil)
				if e != nil {
					t.Fatal(e)
				}
				initial := map[VarID]int64{}
				want := map[VarID]int64{}
				got := map[VarID]int64{}
				for i := 1; i <= n+2; i++ {
					typ := frametransfer.T(frametransfer.Int)
					if widths&(1<<(i-1)) != 0 {
						typ = frametransfer.T(frametransfer.Long)
					}
					id := VarID(i)
					r.Info[id] = ValueInfo{Type: typ, Width: typ.Width(), Origin: ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: i}}
					value := int64(-0x7fffffffffffffff) + int64(i)*0x0101010101010101
					initial[id] = value
					want[id] = value
					got[id] = value
				}
				accept := true
				for _, c := range copies {
					want[c.Dst] = initial[c.Src]
					if ((widths >> int(c.Dst-1)) & 1) != ((widths >> int(c.Src-1)) & 1) {
						accept = false
					}
				}
				next := r.next
				before := map[VarID]ValueInfo{}
				for id, v := range r.Info {
					before[id] = v
				}
				moves, e := r.sequentialize(copies)
				if (e == nil) != accept {
					t.Fatalf("n=%d map=%d widths=%d admitted=%v want=%v copies=%v err=%v", n, number, widths, e == nil, accept, copies, e)
				}
				if !accept {
					refused++
					if moves != nil || r.next != next || !reflect.DeepEqual(r.Info, before) {
						t.Fatal("width refusal mutated method registry")
					}
				} else {
					legal++
					for _, m := range moves {
						if m.Tmp {
							if m.Dst <= VarID(n+2) {
								t.Fatal("temporary overwrote omitted/ambient/constant identity")
							}
							a, b := r.Info[m.Dst], r.Info[m.Src]
							if !a.Temporary || a.Source != m.Src || a.Width != b.Width || !a.Type.Equal(b.Type) || a.Origin != b.Origin {
								t.Fatal("temporary lost original logical metadata")
							}
						}
						v, present := got[m.Src]
						if !present {
							t.Fatal("read a temporary before initialization")
						}
						got[m.Dst] = v
					}
					for id, v := range want {
						if got[id] != v {
							t.Fatalf("simultaneous snapshot violated n=%d map=%d widths=%d id=%d got=%d want=%d moves=%v", n, number, widths, id, got[id], v, moves)
						}
					}
				}
				fmt.Fprintf(inventory, "%d\t%d\t%d\t%t\n", n, number, widths, accept)
			}
		}
	}
	if logical != 1440 || typed != 43612 {
		t.Fatal(logical, typed)
	}
	t.Logf("E02 logical=%d typed=%d legal=%d refused=%d; inventory-sha256=%x", logical, typed, legal, refused, inventory.Sum(nil))
}

func TestPhiLoweringRefusesForgedEqualWidthCategory(t *testing.T) {
	// This is valid original bytecode/SSA. Only the advertised phi category is
	// corrupted; the original edge snapshots retain their integer operands.
	code := []byte{core.OP_ILOAD_0, core.OP_IFEQ, 0, 7, core.OP_ICONST_1, core.OP_GOTO, 0, 4, core.OP_ICONST_2, core.OP_IRETURN}
	for _, bad := range []frametransfer.Type{frametransfer.T(frametransfer.Float), frametransfer.RefOf("java/lang/Object"), frametransfer.T(frametransfer.Null), frametransfer.UninitAt(10)} {
		t.Run(bad.String(), func(t *testing.T) {
			fn, _ := ssaDestroy(t, code, "(I)I", nil)
			if len(fn.Phis) == 0 {
				t.Fatal("fixture lost original join")
			}
			fn.Phis[0].Type = bad
			before := fn.Normalize()
			plan, e := Destroy(fn)
			if e == nil || plan != nil || !strings.HasPrefix(e.Error(), "invalid_input:") {
				t.Fatalf("published forged phi category: plan=%v err=%v", plan != nil, e)
			}
			if fn.Normalize() != before {
				t.Fatal("failure mutated original SSA input")
			}
		})
	}
}
