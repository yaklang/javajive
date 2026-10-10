package core

import "testing"

func TestNestedValueBoundaryRequiresClosedOriginalRouting(t *testing.T) {
	for _, kind := range []string{"closed", "nested", "outside entry", "merge bypass", "backedge", "handler", "handler join", "terminal arm", "switch", "missing instruction", "candidate budget", "protected end", "protected entry", "same protected domain", "missing merge instruction"} {
		t.Run(kind, func(t *testing.T) {
			n := mergeFixture([][]int{{1, 2}, {3}, {3}, nil})
			for _, p := range n {
				p.Instr = &Instruction{OpCode: OP_GOTO}
			}
			n[0].Instr.OpCode = OP_IFNULL
			d := &Decompiler{}
			roots := []*OpCode{n[0]}
			want := kind == "closed" || kind == "nested" || kind == "same protected domain"
			switch kind {
			case "nested":
				inner := &OpCode{CurrentOffset: 15, Instr: &Instruction{OpCode: OP_IFEQ}}
				tail := &OpCode{CurrentOffset: 25, Instr: &Instruction{OpCode: OP_GOTO}}
				n[1].Target = nil
				n[3].Source = n[3].Source[1:]
				LinkOpcode(n[1], inner)
				LinkOpcode(inner, tail)
				LinkOpcode(inner, n[2])
				LinkOpcode(tail, n[3])
			case "outside entry":
				LinkOpcode(&OpCode{CurrentOffset: 5}, n[1])
			case "merge bypass":
				LinkOpcode(&OpCode{CurrentOffset: 5}, n[3])
			case "backedge":
				LinkOpcode(n[1], n[0])
			case "handler":
				n[2].IsCatch = true
			case "handler join":
				n[3].IsTryCatchParent = true
			case "terminal arm":
				n[1].Target = nil
				n[3].Source = n[3].Source[1:]
			case "switch":
				n[0].Instr.OpCode = OP_TABLESWITCH
			case "missing merge instruction":
				n[3].Instr = nil
			case "protected end":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 20, HandlerPc: 50}}
			case "protected entry":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 40, HandlerPc: 50}}
			case "same protected domain":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 40, HandlerPc: 50}}
			case "missing instruction":
				n[1].Instr = nil
			case "candidate budget":
				roots = make([]*OpCode, 129)
				for i := range roots {
					roots[i] = n[0]
				}
			}
			got := d.closedNestedValueRoot(n[3], roots)
			if (got == n[0]) != want {
				t.Fatalf("got=%p want root=%v", got, want)
			}
		})
	}
}
