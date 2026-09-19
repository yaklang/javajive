package core

import (
	"bytes"
	"testing"
)

func TestJSRExpansionUsesRewrittenPCSpace(t *testing.T) {
	code := []byte{OP_ICONST_0, OP_ISTORE_0, OP_JSR, 0, 14, OP_NOP, OP_JSR, 0, 10, OP_NOP, OP_JSR, 0, 6, OP_NOP, OP_ILOAD_0, OP_IRETURN, OP_ASTORE_1, OP_IINC, 0, 1, OP_RET, 1}
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	if err := d.validateControlFlow(); err != nil {
		t.Fatal(err)
	}
	d.inlineJSRSubroutines()
	if d.opcodeCodeLength <= len(code) {
		t.Fatalf("fixture did not expand: %d -> %d", len(code), d.opcodeCodeLength)
	}
	if err := d.validateControlFlow(); err != nil {
		t.Fatal(err)
	}
	for _, op := range d.opCodes {
		if op.Instr.OpCode == OP_JSR || op.Instr.OpCode == OP_RET {
			t.Fatal("subroutine not inlined")
		}
	}
	if _, err := d.buildSemanticCFG(); err != nil {
		t.Fatal(err)
	}
}

func TestJSRFailedExpansionIsTransactional(t *testing.T) {
	// The first leaf can expand; the second noncanonical subroutine falls off
	// the method. A later failure must not commit the successful first round.
	code := []byte{OP_JSR, 0, 7, OP_JSR, 0, 8, OP_RETURN, OP_ASTORE_0, OP_RET, 0, OP_NOP, OP_ASTORE_1, OP_NOP}
	d := NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	original := append([]*OpCode(nil), d.opCodes...)
	operands := make([][]byte, len(original))
	offsets := make([]uint16, len(original))
	for i, op := range original {
		operands[i] = append([]byte(nil), op.Data...)
		offsets[i] = op.CurrentOffset
	}
	length := d.opcodeCodeLength
	d.inlineJSRSubroutines()
	if len(d.opCodes) != len(original) || d.opcodeCodeLength != length {
		t.Fatal("partial expansion committed")
	}
	for i, op := range d.opCodes {
		if op != original[i] || op.CurrentOffset != offsets[i] || !bytes.Equal(op.Data, operands[i]) {
			t.Fatal("failed expansion mutated original opcode")
		}
	}
}
