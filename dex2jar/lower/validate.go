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
	l.delegationIndex = delegation
	for index := range l.method.Instructions {
		instruction := &l.method.Instructions[index]
		if index < delegation {
			if instruction.Target >= 0 {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("branch before this initialization is unsupported"))
			}
			usesThis := false
			for _, register := range instruction.Regs {
				if register == thisRegister {
					usesThis = true
					break
				}
			}
			if usesThis {
				switch {
				case instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09 && instruction.Regs[1] == thisRegister:
					// Aliasing uninitialized this is inert; the alias is only
					// meaningful once it is used, which is checked separately.
				case instruction.Opcode >= 0x59 && instruction.Opcode <= 0x5f && instruction.Regs[1] == thisRegister:
					// DEX permits field stores on uninitialized this before
					// super(). The JVM does not, so these move after delegation;
					// the super constructor cannot observe subclass fields.
					// Moving a store is only value-preserving while the stored
					// register is not redefined before the delegation point.
					valueRegister := instruction.Regs[0]
					for probe := index + 1; probe < delegation; probe++ {
						if _, definitions := l.registerEffects(probe); containsRegister(definitions, valueRegister) {
							return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("field store before delegation reads v%d, which is reassigned before super()", valueRegister))
						}
					}
					l.deferredStores = append(l.deferredStores, index)
				default:
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

func containsRegister(registers []uint16, want uint16) bool {
	for _, register := range registers {
		if register == want {
			return true
		}
	}
	return false
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

// registerEffects comes from the IR's single use/define table so lowering,
// liveness and category analysis cannot drift apart.
func (l *methodLowerer) registerEffects(index int) (uses, definitions []uint16) {
	return l.method.EffectsAt(index)
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
