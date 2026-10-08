package ssabuild

import (
	"math"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func branchSelectedWideLoop() []byte {
	// (flag, first, second, count): branch-specific STOREs meet before
	// a loop which must carry the selected two-word value unchanged.
	return []byte{
		core.OP_ILOAD_0, core.OP_IFEQ, 0, 9,
		core.OP_LLOAD_1, core.OP_LSTORE, 6, core.OP_GOTO, 0, 6,
		core.OP_LLOAD_3, core.OP_LSTORE, 6,
		core.OP_ICONST_0, core.OP_ISTORE, 8,
		core.OP_ILOAD, 8, core.OP_ILOAD, 5, core.OP_IF_ICMPGE, 0, 9,
		core.OP_IINC, 8, 1, core.OP_GOTO, 0xff, 0xf6,
		core.OP_LLOAD, 6, core.OP_LRETURN,
	}
}

func TestPhiCopyCycleConvergesAndRetainsSelectedWideValue(t *testing.T) {
	code := branchSelectedWideLoop()
	ir := irOf(t, code, "(IJJI)J", nil)
	var normalized string
	for _, opt := range []Options{{MaxUpdates: 20000}, {MaxUpdates: 20000, LIFO: true}, {MaxUpdates: 20000, Shuffle: true, ShuffleSeed: 8127}} {
		fn, err := Build(ir, opt)
		if err != nil {
			t.Fatal(err)
		}
		if fn.Work > 500 {
			t.Fatalf("copying loop did not stabilize: %d block visits", fn.Work)
		}
		if normalized == "" {
			normalized = fn.Normalize()
		} else if fn.Normalize() != normalized {
			t.Fatal("worklist order changed SSA equations")
		}
		for _, first := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
			for _, second := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
				for flag := int64(0); flag < 2; flag++ {
					for _, count := range []int64{0, 1, 2, 31, 100} {
						want := second
						if flag != 0 {
							want = first
						}
						got, err := executeObservedSSAParameters(fn, code, map[int]int64{0: flag, 1: first, 3: second, 5: count})
						if err != nil || got != want {
							t.Fatalf("flag=%d first=%d second=%d count=%d: %d want %d error=%v", flag, first, second, count, got, want, err)
						}
					}
				}
			}
		}
	}
}

func TestPhiCopyObservationRejectsSameTypeWrongBranchValue(t *testing.T) {
	code := branchSelectedWideLoop()
	fn := ssaOf(t, irOf(t, code, "(IJJI)J", nil))
	changed := false
	for i := range fn.Phis {
		p := &fn.Phis[i]
		for j := range p.Operands {
			if p.Operands[j].Origin.Kind == OriginParam && p.Operands[j].Origin.Slot == 1 {
				for _, value := range fn.Values {
					if value.Origin == (Origin{Kind: OriginParam, Slot: 3}) {
						p.Operands[j].Val = value.ID
						changed = true
					}
				}
			}
		}
		assertPhiOperands1to1Preds(t, fn, *p)
	}
	if !changed {
		t.Fatal("same-type branch mutation was not applied")
	}
	got, err := executeObservedSSAParameters(fn, code, map[int]int64{0: 1, 1: math.MinInt64, 3: math.MaxInt64, 5: 31})
	if err != nil {
		t.Fatal("mutated model must remain executable", err)
	}
	if got == math.MinInt64 {
		t.Fatal("independent selected-value oracle missed wrong branch operand")
	}
}
