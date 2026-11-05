package lower

import (
	"fmt"

	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

func (l *methodLowerer) validateInvokes() error {
	delegation := -1
	thisRegister := uint16(0)
	isConstructor := l.encoded.Method.Name == "<init>"
	if isConstructor {
		thisRegister = l.method.Code.RegistersSize - l.method.Code.InsSize
	}
	for index := range l.method.Instructions {
		instruction := &l.method.Instructions[index]
		if !isInvoke(instruction.Opcode) {
			continue
		}
		callee := l.file.Methods[instruction.Index]
		if callee.Name == "<clinit>" {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("class initializer uses an invalid invoke form"))
		}
		if callee.Name == "<init>" {
			if !isDirectInvoke(instruction.Opcode) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor uses an invalid invoke form"))
			}
			if allocation, ok := l.allocationByCtor[index]; ok {
				if err := l.validateFusedConstructor(instruction, allocation); err != nil {
					return err
				}
				continue
			}
			if !isConstructor {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is not a fused new-instance allocation", instruction.Regs[0]))
			}
			if instruction.Regs[0] != thisRegister {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is initialized; delegation requires uninitialized this v%d", instruction.Regs[0], thisRegister))
			}
			if callee.Class != l.class.Descriptor && callee.Class != l.class.Superclass {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this may only delegate to %s.<init> or %s.<init>", l.class.Descriptor, l.class.Superclass))
			}
			for _, argument := range instruction.Regs[1:] {
				if argument == thisRegister {
					return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this v%d is also a constructor argument", thisRegister))
				}
			}
			if delegation >= 0 {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor initializes this more than once"))
			}
			delegation = index
			continue
		}
		switch instruction.Opcode {
		case 0x6f, 0x75:
			if callee.Class != l.class.Superclass {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("invoke-super must target a non-constructor method on the direct superclass"))
			}
		case 0x70, 0x76:
			if callee.Class != l.class.Descriptor {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("invoke-direct non-constructor target must belong to the current class"))
			}
			if err := l.validateDefinedDirectTarget(instruction, callee); err != nil {
				return err
			}
		}
	}
	if !isConstructor {
		return nil
	}
	if delegation < 0 {
		return methodDiagnostic(l.method.Context, nil, "constructor", fmt.Errorf("constructor does not initialize this or delegate"))
	}
	return l.validateDelegationShape(delegation, thisRegister)
}

func (l *methodLowerer) validateFusedConstructor(instruction *ir.Instruction, allocation ir.Allocation) error {
	if allocation.Descriptor != l.file.Methods[instruction.Index].Class {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("fused allocation type %q does not match constructor owner %q", allocation.Descriptor, l.file.Methods[instruction.Index].Class))
	}
	for _, alias := range allocation.AliasRegisters {
		if instruction.Regs[0] == alias {
			return nil
		}
	}
	return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is not an alias of its fused new-instance allocation", instruction.Regs[0]))
}

func (l *methodLowerer) validateDelegationShape(delegation int, thisRegister uint16) error {
	delegationOffset := l.method.Instructions[delegation].Offset
	for index := range l.method.Instructions {
		instruction := &l.method.Instructions[index]
		if index < delegation {
			if instruction.Target >= 0 {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("branch before this initialization is unsupported"))
			}
			for _, register := range instruction.Regs {
				if register == thisRegister {
					return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this v%d is used before constructor delegation", thisRegister))
				}
			}
		}
		if index > delegation && instruction.Target >= 0 && uint32(instruction.Target) <= delegationOffset {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("control flow may repeat constructor delegation"))
		}
	}
	return nil
}

func (l *methodLowerer) validateDefinedDirectTarget(instruction *ir.Instruction, callee model.MethodID) error {
	definition, direct, ok := l.definedMethod(callee)
	if !ok {
		return nil
	}
	if !direct || definition.AccessFlags&dexPrivate == 0 || definition.AccessFlags&dexStatic != 0 {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("defined invoke-direct non-constructor target must be a private, non-static direct method"))
	}
	return nil
}

func (l *methodLowerer) definedMethod(callee model.MethodID) (*model.EncodedMethod, bool, bool) {
	for index := range l.class.DirectMethods {
		method := &l.class.DirectMethods[index]
		if sameMethod(method.Method, callee) {
			return method, true, true
		}
	}
	for index := range l.class.VirtualMethods {
		method := &l.class.VirtualMethods[index]
		if sameMethod(method.Method, callee) {
			return method, false, true
		}
	}
	return nil, false, false
}

func sameMethod(left, right model.MethodID) bool {
	return left.Class == right.Class && left.Name == right.Name && left.ProtoIndex == right.ProtoIndex
}

func (l *methodLowerer) validateDefiniteAssignments() error {
	count := int(l.method.Code.RegistersSize)
	entry := make([]bool, count)
	parameterBase := int(l.method.Code.RegistersSize - l.method.Code.InsSize)
	for register := parameterBase; register < count; register++ {
		entry[register] = true
	}
	reachable := make([]bool, len(l.method.Blocks))
	queue := []int{0}
	reachable[0] = true
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		for _, successor := range l.method.Blocks[block].Successors {
			if !reachable[successor] {
				reachable[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	for index, ok := range reachable {
		if !ok {
			instruction := &l.method.Instructions[l.method.Blocks[index].Instructions[0]]
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("unreachable basic block is unsupported"))
		}
	}
	in := make([][]bool, len(l.method.Blocks))
	out := make([][]bool, len(l.method.Blocks))
	for index := range l.method.Blocks {
		in[index] = make([]bool, count)
		out[index] = make([]bool, count)
		if index == 0 {
			copy(in[index], entry)
		} else {
			for register := range in[index] {
				in[index][register] = true
			}
		}
		for register := range out[index] {
			out[index][register] = true
		}
	}
	for pass := 0; pass <= len(l.method.Blocks); pass++ {
		changed := false
		for blockIndex, block := range l.method.Blocks {
			if blockIndex != 0 {
				merged := make([]bool, count)
				for register := range merged {
					merged[register] = true
				}
				for _, predecessor := range block.Predecessors {
					for register := range merged {
						merged[register] = merged[register] && out[predecessor][register]
					}
				}
				if !boolsEqual(in[blockIndex], merged) {
					in[blockIndex] = merged
					changed = true
				}
			}
			state := append([]bool(nil), in[blockIndex]...)
			for _, instructionIndex := range block.Instructions {
				_, definitions := l.registerEffects(instructionIndex)
				for _, register := range definitions {
					state[register] = true
				}
			}
			if !boolsEqual(out[blockIndex], state) {
				out[blockIndex] = state
				changed = true
			}
		}
		if !changed {
			break
		}
		if pass == len(l.method.Blocks) {
			return methodDiagnostic(l.method.Context, nil, "cfg", fmt.Errorf("definite-assignment analysis did not converge"))
		}
	}
	for blockIndex, block := range l.method.Blocks {
		state := append([]bool(nil), in[blockIndex]...)
		for _, instructionIndex := range block.Instructions {
			instruction := &l.method.Instructions[instructionIndex]
			uses, definitions := l.registerEffects(instructionIndex)
			for _, register := range uses {
				if !state[register] {
					return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("v%d may be read before it is assigned", register))
				}
			}
			for _, register := range definitions {
				state[register] = true
			}
		}
	}
	return nil
}

func (l *methodLowerer) registerEffects(index int) (uses, definitions []uint16) {
	instruction := l.method.Instructions[index]
	if _, ok := l.allocationByNew[index]; ok || l.allocationAliasOps[index] {
		return nil, nil
	}
	if allocation, ok := l.allocationByCtor[index]; ok {
		return append([]uint16(nil), instruction.Regs[1:]...), append([]uint16(nil), allocation.AliasRegisters...)
	}
	switch {
	case instruction.Opcode == 0x00 || instruction.Opcode == 0x0e || instruction.Opcode >= 0x28 && instruction.Opcode <= 0x2a:
		return nil, nil
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x03 || instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x0a || instruction.Opcode == 0x0c:
		return nil, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x0f || instruction.Opcode == 0x11 || instruction.Opcode == 0x27:
		return []uint16{instruction.Regs[0]}, nil
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x1c || instruction.Opcode == 0x22:
		return nil, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x1f:
		return []uint16{instruction.Regs[0]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x20 || instruction.Opcode == 0x21 || instruction.Opcode == 0x23:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x37:
		return append([]uint16(nil), instruction.Regs...), nil
	case instruction.Opcode >= 0x38 && instruction.Opcode <= 0x3d:
		return []uint16{instruction.Regs[0]}, nil
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x46:
		return []uint16{instruction.Regs[1], instruction.Regs[2]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0x4b && instruction.Opcode <= 0x4d:
		return []uint16{instruction.Regs[0], instruction.Regs[1], instruction.Regs[2]}, nil
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x58:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0x59 && instruction.Opcode <= 0x5f:
		return []uint16{instruction.Regs[0], instruction.Regs[1]}, nil
	case instruction.Opcode >= 0x60 && instruction.Opcode <= 0x66:
		return nil, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0x67 && instruction.Opcode <= 0x6d:
		return []uint16{instruction.Regs[0]}, nil
	case isInvoke(instruction.Opcode):
		return append([]uint16(nil), instruction.Regs...), nil
	case instruction.Opcode == 0x7b || instruction.Opcode == 0x7c || instruction.Opcode >= 0x8d && instruction.Opcode <= 0x8f:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0x9a:
		return []uint16{instruction.Regs[1], instruction.Regs[2]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xba:
		return []uint16{instruction.Regs[0], instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	default:
		return append([]uint16(nil), instruction.Regs...), nil
	}
}

func boolsEqual(left, right []bool) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
