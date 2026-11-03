package classfilebuilder

import (
	"fmt"
	"math"
)

type stackEffect struct{ pop, push int }

func (c *Code) calculateFrames(descriptor string, access uint16, labels map[*Label]int, pcs []int) (uint16, uint16, error) {
	args, _, err := methodDescriptorWords(descriptor)
	if err != nil {
		return 0, 0, err
	}
	locals := args
	if access&ACC_STATIC == 0 {
		locals++
	}
	instructions := make([]*instruction, 0, len(pcs))
	pcIndex := make(map[int]int, len(pcs))
	for _, node := range c.nodes {
		if node.insn != nil {
			pcIndex[pcs[len(instructions)]] = len(instructions)
			instructions = append(instructions, node.insn)
		}
	}
	for _, insn := range instructions {
		switch insn.kind {
		case kindLocal:
			if insn.arg+insn.arg2 > locals {
				locals = insn.arg + insn.arg2
			}
		case kindIInc:
			if insn.arg+1 > locals {
				locals = insn.arg + 1
			}
		}
	}
	if locals > math.MaxUint16 {
		return 0, 0, fmt.Errorf("max_locals %d exceeds 65535", locals)
	}
	if len(instructions) == 0 {
		return 0, uint16(locals), nil
	}
	depths := make([]int, len(instructions))
	for i := range depths {
		depths[i] = -1
	}
	queue := make([]int, 0, len(instructions))
	enqueue := func(index, depth int) error {
		if index < 0 || index >= len(instructions) {
			return fmt.Errorf("control flow target is not an instruction")
		}
		if depths[index] < 0 {
			depths[index] = depth
			queue = append(queue, index)
			return nil
		}
		if depths[index] != depth {
			return fmt.Errorf("inconsistent stack height at bytecode offset %d: %d and %d", pcs[index], depths[index], depth)
		}
		return nil
	}
	if err := enqueue(0, 0); err != nil {
		return 0, 0, err
	}
	for i, handler := range c.handlers {
		pc, ok := labels[handler.handler]
		if !ok {
			return 0, 0, fmt.Errorf("exception handler %d has unbound handler label", i)
		}
		index, ok := pcIndex[pc]
		if !ok {
			return 0, 0, fmt.Errorf("exception handler %d does not start at an instruction", i)
		}
		if err := enqueue(index, 1); err != nil {
			return 0, 0, err
		}
	}
	maxStack := 0
	for len(queue) > 0 {
		index := queue[0]
		queue = queue[1:]
		insn := instructions[index]
		effect, err := c.instructionEffect(insn)
		if err != nil {
			return 0, 0, fmt.Errorf("bytecode offset %d: %w", pcs[index], err)
		}
		before := depths[index]
		if before < effect.pop {
			return 0, 0, fmt.Errorf("stack underflow at bytecode offset %d: need %d words, have %d", pcs[index], effect.pop, before)
		}
		after := before - effect.pop + effect.push
		if before > maxStack {
			maxStack = before
		}
		if after > maxStack {
			maxStack = after
		}
		if after > math.MaxUint16 {
			return 0, 0, fmt.Errorf("max_stack %d exceeds 65535", after)
		}
		if insn.kind == kindBranch {
			targetPC := labels[insn.target]
			targetIndex, ok := pcIndex[targetPC]
			if !ok {
				return 0, 0, fmt.Errorf("branch target at offset %d is not an instruction", targetPC)
			}
			if err := enqueue(targetIndex, after); err != nil {
				return 0, 0, err
			}
			if insn.op == GOTO {
				continue
			}
		}
		if isTerminal(insn.op) {
			continue
		}
		if index+1 >= len(instructions) {
			return 0, 0, fmt.Errorf("reachable execution falls off end of Code after offset %d", pcs[index])
		}
		if err := enqueue(index+1, after); err != nil {
			return 0, 0, err
		}
	}
	return uint16(maxStack), uint16(locals), nil
}

func (c *Code) instructionEffect(i *instruction) (stackEffect, error) {
	switch i.kind {
	case kindLocal:
		width := i.arg2
		if i.op >= ILOAD && i.op <= ALOAD {
			return stackEffect{push: width}, nil
		}
		return stackEffect{pop: width}, nil
	case kindIInc:
		return stackEffect{}, nil
	case kindLDC:
		return stackEffect{push: 1}, nil
	case kindByte:
		if i.op == BIPUSH {
			return stackEffect{push: 1}, nil
		}
		return stackEffect{pop: 1, push: 1}, nil
	case kindShort:
		return stackEffect{push: 1}, nil
	case kindBranch:
		if i.op == GOTO {
			return stackEffect{}, nil
		}
		if i.op >= IF_ICMPEQ && i.op <= IF_ACMPNE {
			return stackEffect{pop: 2}, nil
		}
		return stackEffect{pop: 1}, nil
	case kindCP:
		entry, err := c.pool.entry(uint16(i.arg))
		if err != nil {
			return stackEffect{}, err
		}
		switch i.op {
		case GETSTATIC, PUTSTATIC, GETFIELD, PUTFIELD:
			words, err := fieldDescriptorWords(entry.text)
			if err != nil {
				return stackEffect{}, err
			}
			switch i.op {
			case GETSTATIC:
				return stackEffect{push: words}, nil
			case PUTSTATIC:
				return stackEffect{pop: words}, nil
			case GETFIELD:
				return stackEffect{pop: 1, push: words}, nil
			default:
				return stackEffect{pop: 1 + words}, nil
			}
		case INVOKEVIRTUAL, INVOKESPECIAL, INVOKESTATIC:
			args, result, err := methodDescriptorWords(entry.text)
			if err != nil {
				return stackEffect{}, err
			}
			if i.op != INVOKESTATIC {
				args++
			}
			return stackEffect{pop: args, push: result}, nil
		case NEW:
			return stackEffect{push: 1}, nil
		case ANEWARRAY:
			return stackEffect{pop: 1, push: 1}, nil
		case CHECKCAST:
			return stackEffect{pop: 1, push: 1}, nil
		case INSTANCEOF:
			return stackEffect{pop: 1, push: 1}, nil
		}
	case kindInvokeInterface:
		entry, err := c.pool.entry(uint16(i.arg))
		if err != nil {
			return stackEffect{}, err
		}
		args, result, err := methodDescriptorWords(entry.text)
		if err != nil {
			return stackEffect{}, err
		}
		return stackEffect{pop: args + 1, push: result}, nil
	case kindSimple:
		if effect, ok := simpleEffect(i.op); ok {
			return effect, nil
		}
	}
	return stackEffect{}, fmt.Errorf("unsupported stack effect for opcode 0x%02x", byte(i.op))
}

func simpleEffect(op Opcode) (stackEffect, bool) {
	switch op {
	case NOP:
		return stackEffect{}, true
	case ACONST_NULL, ICONST_M1, ICONST_0, ICONST_1, ICONST_2, ICONST_3, ICONST_4, ICONST_5, FCONST_0, FCONST_1, FCONST_2:
		return stackEffect{push: 1}, true
	case LCONST_0, LCONST_1, DCONST_0, DCONST_1:
		return stackEffect{push: 2}, true
	case IALOAD, FALOAD, AALOAD, BALOAD, CALOAD, SALOAD:
		return stackEffect{pop: 2, push: 1}, true
	case LALOAD, DALOAD:
		return stackEffect{pop: 2, push: 2}, true
	case IASTORE, FASTORE, AASTORE, BASTORE, CASTORE, SASTORE:
		return stackEffect{pop: 3}, true
	case LASTORE, DASTORE:
		return stackEffect{pop: 4}, true
	case POP:
		return stackEffect{pop: 1}, true
	case POP2:
		return stackEffect{pop: 2}, true
	case DUP:
		return stackEffect{pop: 1, push: 2}, true
	case SWAP:
		return stackEffect{pop: 2, push: 2}, true
	case IADD, FADD, ISUB, FSUB, IMUL, FMUL, IDIV, FDIV, IREM, FREM, ISHL, ISHR, IUSHR, IAND, IOR, IXOR:
		return stackEffect{pop: 2, push: 1}, true
	case LADD, DADD, LSUB, DSUB, LMUL, DMUL, LDIV, DDIV, LREM, DREM, LAND, LOR, LXOR:
		return stackEffect{pop: 4, push: 2}, true
	case LSHL, LSHR, LUSHR:
		return stackEffect{pop: 3, push: 2}, true
	case INEG, FNEG, I2B, I2C, I2S:
		return stackEffect{pop: 1, push: 1}, true
	case LNEG, DNEG:
		return stackEffect{pop: 2, push: 2}, true
	case I2L, I2D, F2L, F2D:
		return stackEffect{pop: 1, push: 2}, true
	case I2F, F2I:
		return stackEffect{pop: 1, push: 1}, true
	case L2I, L2F, D2I, D2F:
		return stackEffect{pop: 2, push: 1}, true
	case L2D, D2L:
		return stackEffect{pop: 2, push: 2}, true
	case LCMP, DCMPL, DCMPG:
		return stackEffect{pop: 4, push: 1}, true
	case FCMPL, FCMPG:
		return stackEffect{pop: 2, push: 1}, true
	case IRETURN, FRETURN, ARETURN, ATHROW, MONITORENTER, MONITOREXIT:
		return stackEffect{pop: 1}, true
	case LRETURN, DRETURN:
		return stackEffect{pop: 2}, true
	case RETURN:
		return stackEffect{}, true
	case ARRAYLENGTH:
		return stackEffect{pop: 1, push: 1}, true
	default:
		return stackEffect{}, false
	}
}

func isTerminal(op Opcode) bool {
	return op >= IRETURN && op <= RETURN || op == ATHROW
}
