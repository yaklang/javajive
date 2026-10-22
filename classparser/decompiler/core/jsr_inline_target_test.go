package core

import "testing"

func TestBranchAbsoluteTargetKeepsWideOffset(t *testing.T) {
	wide := &OpCode{
		CurrentOffset: 10,
		Instr:         &Instruction{OpCode: OP_GOTO_W},
		Data:          []byte{0x00, 0x00, 0x01, 0x00},
	}
	dst, ok := branchAbsoluteTarget(wide)
	if !ok || dst != 10+256 {
		t.Fatalf("wide target = %d ok=%v", dst, ok)
	}
	narrow := &OpCode{
		CurrentOffset: 20,
		Instr:         &Instruction{OpCode: OP_JSR},
		Data:          []byte{0xff, 0xfe},
	}
	dst, ok = branchAbsoluteTarget(narrow)
	if !ok || dst != 20-2 {
		t.Fatalf("jsr target = %d ok=%v", dst, ok)
	}
	if _, ok := branchAbsoluteTarget(&OpCode{CurrentOffset: 1, Instr: &Instruction{OpCode: OP_JSR_W}, Data: []byte{1, 2}}); ok {
		t.Fatal("truncated jsr_w operand was accepted")
	}
}
