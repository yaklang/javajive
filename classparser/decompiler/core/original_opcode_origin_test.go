package core

import "testing"

func TestOriginalOpcodeLookupExcludesSyntheticPcCollision(t *testing.T) {
	for _, scenario := range []string{"only synthetic", "original zero", "original nonzero", "ambiguous originals", "missing instruction"} {
		t.Run(scenario, func(t *testing.T) {
			start, end, entry, real := op(OP_START, 0), op(OP_END, 0), op(OP_TRY_CATCH, 0), op(OP_NEW, 0)
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{start: nil, end: nil, entry: nil}}
			var want *OpCode
			offset := 0
			switch scenario {
			case "original zero":
				d.opcodeToSimulateStack[real] = nil
				want = real
			case "original nonzero":
				real.CurrentOffset = 7
				offset = 7
				d.opcodeToSimulateStack[real] = nil
				want = real
			case "ambiguous originals":
				d.opcodeToSimulateStack[real] = nil
				d.opcodeToSimulateStack[op(OP_INVOKESTATIC, 0)] = nil
			case "missing instruction":
				d.opcodeToSimulateStack[&OpCode{}] = nil
			}
			for i := 0; i < 512; i++ {
				if got := d.opcodeAtOffset(offset); got != want {
					t.Fatalf("original witness=%p want=%p iteration=%d", got, want, i)
				}
			}
		})
	}
}
