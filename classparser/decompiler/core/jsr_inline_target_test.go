package core

import (
	"encoding/binary"
	"testing"
)

func TestInlineJSRAcrossSwitchAndWideBranches(t *testing.T) {
	for _, opcode := range []int{OP_TABLESWITCH, OP_LOOKUPSWITCH} {
		code := []byte{byte(OP_ICONST_0), byte(opcode), 0, 0}
		word := func(value uint32) { code = binary.BigEndian.AppendUint32(code, value) }
		word(19) // default, relative to the switch at PC 1: jsr at PC 20
		if opcode == OP_TABLESWITCH {
			word(0)
			word(0)
			word(19) // low, high, one target
		} else {
			word(1)
			word(0)
			word(19) // one key/target pair
		}
		code = append(code, byte(OP_JSR), 0, 4, byte(OP_RETURN), byte(OP_ASTORE_1), byte(OP_RET), 1)
		t.Run(InstrInfos[opcode].Name, func(t *testing.T) { assertJSRFullyInlined(t, code) })
	}
	t.Run("jsr_w", func(t *testing.T) {
		assertJSRFullyInlined(t, []byte{byte(OP_JSR_W), 0, 0, 0, 6, byte(OP_RETURN), byte(OP_ASTORE_1), byte(OP_RET), 1})
	})
	t.Run("goto_w in subroutine", func(t *testing.T) {
		assertJSRFullyInlined(t, []byte{byte(OP_JSR), 0, 4, byte(OP_RETURN), byte(OP_ASTORE_1), byte(OP_GOTO_W), 0, 0, 0, 5, byte(OP_RET), 1})
	})
}

func assertJSRFullyInlined(t *testing.T, code []byte) {
	t.Helper()
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	if err := d.inlineJSRSubroutines(); err != nil {
		t.Fatal(err)
	}
	for _, op := range d.Opcodes() {
		if op.Instr.OpCode == OP_JSR || op.Instr.OpCode == OP_JSR_W || op.Instr.OpCode == OP_RET {
			t.Fatalf("subroutine survived at PC %d", op.CurrentOffset)
		}
	}
	if err := d.validateControlFlow(); err != nil {
		t.Fatalf("remapped control flow: %v", err)
	}
	if _, err := d.BuildSemanticCFG(); err != nil {
		t.Fatalf("semantic CFG: %v", err)
	}
}

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
