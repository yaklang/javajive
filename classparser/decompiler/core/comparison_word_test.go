package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

// This is a bounded control-flow certificate test, not an end-to-end truth
// oracle. A result with any other consumer retains its numeric category. The
// production JVM bank independently checks both paths of the optimization.
func TestBoundedComparisonWordConsumerCertificate(t *testing.T) {
	cases := 0
	for _, cmp := range []int{OP_LCMP, OP_FCMPL, OP_FCMPG, OP_DCMPL, OP_DCMPG} {
		for _, branch := range []int{OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE} {
			for pad := 0; pad <= 34; pad++ {
				for _, fault := range []string{"none", "alternate-entry", "fork", "back-edge", "missing-instruction", "wrong-predecessor", "stored-word", "duplicated-word", "return-word", "effect-before-consumer", "no-successor"} {
					first := &OpCode{Instr: InstrInfos[cmp], CurrentOffset: 10}
					prev := first
					var chain []*OpCode
					for i := 0; i <= pad; i++ {
						kind := OP_NOP
						if i == pad {
							kind = branch
						}
						next := &OpCode{Instr: InstrInfos[kind], CurrentOffset: uint16(11 + i), Source: []*OpCode{prev}}
						prev.Target = []*OpCode{next}
						chain = append(chain, next)
						prev = next
					}
					target := chain[pad]
					switch fault {
					case "alternate-entry":
						target.Source = append(target.Source, &OpCode{})
					case "fork":
						first.Target = append(first.Target, &OpCode{})
					case "back-edge":
						target.CurrentOffset = 9
					case "missing-instruction":
						target.Instr = nil
					case "wrong-predecessor":
						target.Source = []*OpCode{{}}
					case "stored-word":
						target.Instr = InstrInfos[OP_ISTORE]
					case "duplicated-word":
						target.Instr = InstrInfos[OP_DUP]
					case "return-word":
						target.Instr = InstrInfos[OP_IRETURN]
					case "effect-before-consumer":
						target.Instr = InstrInfos[OP_INVOKESTATIC]
					case "no-successor":
						first.Target = nil
					}
					want := fault == "none" && pad < 32
					if got := comparisonHasDirectZeroBranch(first); got != want {
						t.Fatalf("cmp%d branch%d pad%d fault%s: %v want%v", cmp, branch, pad, fault, got, want)
					}
					cases++
				}
			}
		}
	}
	if comparisonHasDirectZeroBranch(nil) {
		t.Fatal("missing producer authorized")
	}
	if cases != 11550 {
		t.Fatal("incomplete graph bank", cases)
	}
	t.Logf("bounded comparison consumer graphs=%d", cases)
}

func TestComparisonWordInputsRetainIndependentOriginalCaptures(t *testing.T) {
	for _, kind := range []int{OP_LCMP, OP_FCMPL, OP_FCMPG, OP_DCMPL, OP_DCMPG} {
		typ := types.JavaFloat
		if kind == OP_LCMP {
			typ = types.JavaLong
		} else if kind == OP_DCMPL || kind == OP_DCMPG {
			typ = types.JavaDouble
		}
		left := values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaPrimer(typ) })
		right := values.NewCustomValue(nil, func() types.JavaType { return types.NewJavaPrimer(typ) })
		sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId().Next())
		d := &Decompiler{}
		op := &OpCode{Instr: InstrInfos[kind], CurrentOffset: 23}
		a, b := d.comparisonWordOperands(sim, op, left, right)
		ar, br := a.(*values.JavaRef), b.(*values.JavaRef)
		if ar == br || values.SameLocal(ar, br) || ar.Val != left || br.Val != right {
			t.Fatal("captures aliased/reversed")
		}
		packet := d.comparisonWordInputs[op]
		if len(packet) != 2 || packet[0].LeftValue != ar || packet[0].JavaValue != left || packet[1].LeftValue != br || packet[1].JavaValue != right {
			t.Fatal("capture emission order lost")
		}
		if len(d.disFoldRef) != 2 || d.disFoldRef[0] != ar || d.disFoldRef[1] != br {
			t.Fatal("capture may be folded and reevaluated")
		}
		for i, ref := range []*values.JavaRef{ar, br} {
			v := []values.JavaValue{left, right}[i]
			pc, opkind, ok := ref.OriginalStackMaterializationWitness(v)
			if !ok || pc != 23 || opkind != kind || ref.Type().String(nil) != typ {
				t.Fatal("original capture/type witness changed")
			}
			if _, _, ok := ref.OriginalStackMaterializationWitness([]values.JavaValue{right, left}[i]); ok {
				t.Fatal("different producer authorized")
			}
		}
	}
}
