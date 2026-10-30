package core

import (
	"encoding/binary"
	"testing"
)

func TestInlineJSRAcrossSwitchAndWideBranches(t *testing.T) {
	for name, code := range jsrInlineFixtures() {
		code := code
		t.Run(name, func(t *testing.T) { assertJSRFullyInlined(t, code) })
	}
}

// JDEC_JSR_WIDE_INLINE_OFF must restore the pre-fix bail for switch and wide
// targets while leaving the narrow jsr path inlined.
func TestJSRWideInlineIsLoadBearing(t *testing.T) {
	fixtures := jsrInlineFixtures()
	t.Setenv("JDEC_JSR_WIDE_INLINE_OFF", "1")
	if jsrSurvives(t, fixtures["narrow jsr"]) {
		t.Fatal("narrow jsr must still inline with the kill-switch set")
	}
	for _, name := range []string{InstrInfos[OP_TABLESWITCH].Name, InstrInfos[OP_LOOKUPSWITCH].Name, "jsr_w", "goto_w in subroutine"} {
		if !jsrSurvives(t, fixtures[name]) {
			t.Fatalf("%s: kill-switch did not restore the pre-fix bail", name)
		}
	}
	t.Setenv("JDEC_JSR_WIDE_INLINE_OFF", "")
	for name, code := range fixtures {
		if jsrSurvives(t, code) {
			t.Fatalf("%s: switch ON left a subroutine", name)
		}
	}
}

func jsrInlineFixtures() map[string][]byte {
	fixtures := map[string][]byte{}
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
		fixtures[InstrInfos[opcode].Name] = code
	}
	fixtures["narrow jsr"] = []byte{byte(OP_JSR), 0, 4, byte(OP_RETURN), byte(OP_ASTORE_1), byte(OP_RET), 1}
	fixtures["jsr_w"] = []byte{byte(OP_JSR_W), 0, 0, 0, 6, byte(OP_RETURN), byte(OP_ASTORE_1), byte(OP_RET), 1}
	fixtures["goto_w in subroutine"] = []byte{byte(OP_JSR), 0, 4, byte(OP_RETURN), byte(OP_ASTORE_1), byte(OP_GOTO_W), 0, 0, 0, 5, byte(OP_RET), 1}
	return fixtures
}

func jsrSurvives(t *testing.T, code []byte) bool {
	t.Helper()
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	if err := d.inlineJSRSubroutines(); err != nil {
		t.Fatal(err)
	}
	for _, op := range d.Opcodes() {
		switch op.Instr.OpCode {
		case OP_JSR, OP_JSR_W, OP_RET:
			return true
		}
	}
	return false
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
