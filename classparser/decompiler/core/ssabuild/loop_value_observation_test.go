package ssabuild

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Execute produced SSA values and incoming-edge phi operands, not the source
// bytecode interpreter. The independently known simultaneous swap is the
// observable oracle. This closes the gap where original bytecode parity and
// phi arity both pass despite a wrong value on one predecessor.
func TestT13LoopPhiValuesFollowActualIncomingEdge(t *testing.T) {
	for _, wide := range []bool{false, true} {
		code, desc := phiSwapBytecode(), "(I)I"
		if wide {
			code, desc = phiSwapWideBytecode(), "(I)J"
		}
		ir := irOf(t, code, desc, nil)
		for _, opt := range []Options{{}, {LIFO: true}, {Shuffle: true, ShuffleSeed: 1701}} {
			fn, err := Build(ir, opt)
			if err != nil {
				t.Fatal(err)
			}
			for n := 0; n <= 100; n++ {
				got, err := executeObservedSSAValues(fn, code, n)
				if err != nil {
					t.Fatalf("wide=%v options=%+v n=%d: %v", wide, opt, n, err)
				}
				want := int64(12)
				if n%2 == 1 {
					want = 21
				}
				if wide {
					want = int64(1 - n%2)
				}
				if got != want {
					t.Fatalf("wide=%v options=%+v n=%d got=%d want=%d", wide, opt, n, got, want)
				}
			}
		}
	}
}

// This model substitutes a defined entry value on the loop's carried edge.
// Both predecessor keys and phi arity remain correct, and the original JVM
// instructions are unchanged. Shape checks and a bytecode-only interpreter
// would miss it; the produced-value observer must reject its numerical result.
// It is an SSA model mutant, not an executed Java compilation mutant.
func TestT13LoopPhiObservationRejectsSameArityWrongCarriedValue(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprintf("wide=%v", wide), func(t *testing.T) {
			code, desc, want := phiSwapBytecode(), "(I)I", int64(21)
			if wide {
				code, desc, want = phiSwapWideBytecode(), "(I)J", 0
			}
			fn, err := Build(irOf(t, code, desc, nil), Options{})
			if err != nil {
				t.Fatal(err)
			}
			changed := 0
			for i := range fn.Phis {
				p := &fn.Phis[i]
				if !p.Slot.Local || p.Slot.Index != 1 {
					continue
				}
				var initial ValueID
				for _, input := range p.Operands {
					if input.Origin.Kind == OriginInstr && input.Origin.PC == 0 {
						initial = input.Val
					}
				}
				if initial == 0 {
					continue
				}
				for j := range p.Operands {
					if p.Operands[j].Val != initial {
						p.Operands[j].Val = initial
						changed++
					}
				}
				assertPhiOperands1to1Preds(t, fn, *p)
			}
			if changed == 0 {
				t.Fatal("model did not substitute a carried value")
			}
			got, err := executeObservedSSAValues(fn, code, 1)
			if err != nil {
				t.Fatalf("model must remain executable, not fail through an unbound operand: %v", err)
			}
			if got == want {
				t.Fatal("independent loop result failed to detect wrong carried value")
			}
		})
	}
}

func executeObservedSSAValues(fn *Function, code []byte, n int) (int64, error) {
	return executeObservedSSAParameters(fn, code, map[int]int64{0: int64(n)})
}

func executeObservedSSAParameters(fn *Function, code []byte, parameters map[int]int64) (int64, error) {
	env := map[ValueID]int64{}
	for _, v := range fn.Values {
		if v.Origin.Kind == OriginParam {
			value, known := parameters[v.Origin.Slot]
			if !known {
				return 0, fmt.Errorf("unexpected param")
			}
			env[v.ID] = value
		}
	}
	records := map[int]InstructionValues{}
	for _, r := range fn.Instructions {
		records[int(r.PC)] = r
	}
	block := map[int][]Phi{}
	for _, b := range fn.Blocks {
		block[int(b.First)] = fn.PhisOf(b.ID)
	}
	prev, pc := -1, 0
	for steps := 0; steps < 10000; steps++ {
		pending := map[ValueID]int64{}
		for _, p := range block[pc] {
			matched := 0
			for _, operand := range p.Operands {
				if prev < 0 && operand.Edge.Kind == EntryEdgeKind || prev >= 0 && int(operand.Edge.From) == prev {
					value, ok := env[operand.Val]
					if !ok {
						return 0, fmt.Errorf("undefined phi operand pc=%d value=%d origin=%+v", pc, operand.Val, operand.Origin)
					}
					pending[p.ID] = value
					matched++
				}
			}
			if matched != 1 {
				return 0, fmt.Errorf("phi pc=%d predecessor=%d matches=%d", pc, prev, matched)
			}
		}
		for id, value := range pending {
			env[id] = value
		}
		r, ok := records[pc]
		if !ok {
			return 0, fmt.Errorf("missing instruction pc=%d", pc)
		}
		uses := make([]int64, len(r.UseIDs))
		for i, id := range r.UseIDs {
			v, ok := env[id]
			if !ok {
				return 0, fmt.Errorf("undefined use pc=%d value=%d", pc, id)
			}
			uses[i] = v
		}
		result := func(v int64) error {
			if len(r.ResultIDs) != 1 {
				return fmt.Errorf("result arity pc=%d: %v", pc, r.ResultIDs)
			}
			env[r.ResultIDs[0]] = v
			return nil
		}
		next := pc + 1
		var err error
		op := code[pc]
		switch {
		case op >= core.OP_ICONST_M1 && op <= core.OP_ICONST_5:
			err = result(int64(op) - int64(core.OP_ICONST_0))
		case op == core.OP_LCONST_0 || op == core.OP_LCONST_1:
			err = result(int64(op - core.OP_LCONST_0))
		case op == core.OP_BIPUSH:
			err = result(int64(int8(code[pc+1])))
			next = pc + 2
		case op == core.OP_IINC:
			if len(uses) != 1 {
				return 0, fmt.Errorf("iinc uses")
			}
			err = result(int64(int32(uses[0] + int64(int8(code[pc+2])))))
			next = pc + 3
		case op == core.OP_IADD:
			if len(uses) != 2 {
				return 0, fmt.Errorf("add uses")
			}
			err = result(int64(int32(uses[0] + uses[1])))
		case op == core.OP_IMUL:
			if len(uses) != 2 {
				return 0, fmt.Errorf("mul uses")
			}
			err = result(int64(int32(uses[0] * uses[1])))
		case op == core.OP_GOTO:
			next = pc + int(int16(binary.BigEndian.Uint16(code[pc+1:pc+3])))
		case op == core.OP_IF_ICMPLT:
			if len(uses) != 2 {
				return 0, fmt.Errorf("branch uses")
			}
			next = pc + 3
			if uses[0] < uses[1] {
				next = pc + int(int16(binary.BigEndian.Uint16(code[pc+1:pc+3])))
			}
		case op == core.OP_IF_ICMPGE:
			if len(uses) != 2 {
				return 0, fmt.Errorf("branch uses")
			}
			next = pc + 3
			if uses[0] >= uses[1] {
				next = pc + int(int16(binary.BigEndian.Uint16(code[pc+1:pc+3])))
			}
		case op == core.OP_IFEQ:
			if len(uses) != 1 {
				return 0, fmt.Errorf("branch uses")
			}
			next = pc + 3
			if uses[0] == 0 {
				next = pc + int(int16(binary.BigEndian.Uint16(code[pc+1:pc+3])))
			}
		case op == core.OP_IRETURN || op == core.OP_LRETURN:
			if len(uses) != 1 {
				return 0, fmt.Errorf("return uses")
			}
			return uses[0], nil
		case op == core.OP_ILOAD || op == core.OP_ISTORE || op == core.OP_LLOAD || op == core.OP_LSTORE:
			next = pc + 2
		case op >= core.OP_ILOAD_0 && op <= core.OP_LLOAD_3 || op >= core.OP_ISTORE_0 && op <= core.OP_LSTORE_3:
		default:
			return 0, fmt.Errorf("unsupported reviewed opcode %02x pc=%d", op, pc)
		}
		if err != nil {
			return 0, err
		}
		prev, pc = pc, next
	}
	return 0, fmt.Errorf("bounded trace did not return")
}
