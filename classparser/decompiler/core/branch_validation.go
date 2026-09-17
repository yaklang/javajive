package core

import (
	"encoding/binary"
	"fmt"
)

// BranchTarget decodes a signed JVM branch displacement without uint16 wraparound.
func BranchTarget(pc int, operand []byte, width, codeLen int) (int, error) {
	var delta int64
	switch width {
	case 2:
		if len(operand) < 2 {
			return 0, fmt.Errorf("truncated branch16 at PC %d", pc)
		}
		delta = int64(int16(binary.BigEndian.Uint16(operand)))
	case 4:
		if len(operand) < 4 {
			return 0, fmt.Errorf("truncated branch32 at PC %d", pc)
		}
		delta = int64(int32(binary.BigEndian.Uint32(operand)))
	default:
		return 0, fmt.Errorf("unsupported branch width %d", width)
	}
	target := int64(pc) + delta
	if pc < 0 || pc >= codeLen || target < 0 || target >= int64(codeLen) {
		return 0, fmt.Errorf("branch target out of Code: PC %d displacement %d", pc, delta)
	}
	return int(target), nil
}

// Validate all encoded edges, including unreachable instructions, before graph construction.
func (d *Decompiler) validateControlFlow() error {
	boundary := func(pc int64) error {
		if pc < 0 || pc >= int64(len(d.bytecodes)) {
			return fmt.Errorf("target PC %d out of Code", pc)
		}
		if _, ok := d.offsetToOpcodeIndex[uint16(pc)]; !ok {
			return fmt.Errorf("target PC %d is not an instruction boundary", pc)
		}
		return nil
	}
	for _, op := range d.opCodes {
		width := 0
		switch op.Instr.OpCode {
		case OP_GOTO, OP_JSR, OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
			width = 2
		case OP_GOTO_W, OP_JSR_W:
			width = 4
		case OP_TABLESWITCH, OP_LOOKUPSWITCH:
			if err := boundary(int64(op.SwitchDefaultOffset)); err != nil {
				return fmt.Errorf("switch at PC %d: %w", op.CurrentOffset, err)
			}
			var invalid error
			op.SwitchJmpCase.ForEach(func(key int, target int32) bool { invalid = boundary(int64(target)); return invalid == nil })
			if invalid != nil {
				return fmt.Errorf("switch at PC %d: %w", op.CurrentOffset, invalid)
			}
		}
		if width > 0 {
			target, err := BranchTarget(int(op.CurrentOffset), op.Data, width, len(d.bytecodes))
			if err != nil {
				return err
			}
			if err = boundary(int64(target)); err != nil {
				return err
			}
			op.BranchTarget = target
		}
	}
	for _, entry := range d.ExceptionTable {
		if entry.StartPc >= entry.EndPc || int(entry.EndPc) > len(d.bytecodes) {
			return fmt.Errorf("invalid exception range %d..%d", entry.StartPc, entry.EndPc)
		}
		for _, pc := range []uint16{entry.StartPc, entry.HandlerPc} {
			if err := boundary(int64(pc)); err != nil {
				return fmt.Errorf("exception table: %w", err)
			}
		}
		if int(entry.EndPc) != len(d.bytecodes) {
			if err := boundary(int64(entry.EndPc)); err != nil {
				return fmt.Errorf("exception table end: %w", err)
			}
		}
	}
	return nil
}
