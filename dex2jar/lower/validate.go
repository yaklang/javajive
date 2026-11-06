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
				if allocation.Fused {
					if err := l.validateFusedConstructor(instruction, allocation); err != nil {
						return err
					}
				}
				continue
			}
			if !isConstructor {
				// A conditional constructor call may initialize an allocation on
				// a different path; the receiver still aliases that allocation
				// and its static type names the class being initialized.
				if l.isAllocationAlias(instruction.Regs[0], callee.Class) {
					continue
				}
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is not a fused new-instance allocation", instruction.Regs[0]))
			}
			// The receiver may name any register that still aliases uninitialized
			// this; d8 moves this into a low register for range invokes.
			aliases := l.thisAliasSet(index, thisRegister)
			if !aliases[instruction.Regs[0]] {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is initialized; delegation requires uninitialized this v%d", instruction.Regs[0], thisRegister))
			}
			if callee.Class != l.class.Descriptor && callee.Class != l.class.Superclass {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this may only delegate to %s.<init> or %s.<init>", l.class.Descriptor, l.class.Superclass))
			}
			for _, argument := range instruction.Regs[1:] {
				if aliases[argument] {
					return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this v%d is also a constructor argument", argument))
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
			if !l.isSuperclass(callee.Class) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("invoke-super target %q is not a superclass of %q", callee.Class, l.class.Descriptor))
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

// isSuperclass reports whether target is an ancestor of the current class.
// Invoke-super may name an inherited method declared further up the chain than
// the direct superclass. A type declared outside the application counts as an
// ancestor: its hierarchy is not in the DEX, and the input was verified against
// the platform classpath. An application target must be reached through the
// declared superclass chain, because a platform ancestor cannot extend an
// application class.
func (l *methodLowerer) isSuperclass(target string) bool {
	if target == l.class.Superclass {
		return true
	}
	index := classHierarchy(l.file)
	if _, ok := index[target]; !ok {
		return true
	}
	visited := map[string]bool{}
	for current := l.class.Superclass; current != "" && !visited[current]; {
		visited[current] = true
		if current == target {
			return true
		}
		class, ok := index[current]
		if !ok {
			return false
		}
		current = class.Superclass
	}
	return false
}

// thisAliasSet returns the registers that alias uninitialized this after the
// instructions before limit. A move-object from a known alias creates one;
// defining the register with anything else destroys it.
func (l *methodLowerer) thisAliasSet(limit int, thisRegister uint16) map[uint16]bool {
	aliases := map[uint16]bool{thisRegister: true}
	for index := 0; index < limit && index < len(l.method.Instructions); index++ {
		instruction := &l.method.Instructions[index]
		if instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09 && aliases[instruction.Regs[1]] {
			aliases[instruction.Regs[0]] = true
			continue
		}
		_, definitions := l.method.EffectsAt(index)
		for _, register := range definitions {
			if register != thisRegister {
				delete(aliases, register)
			}
		}
	}
	return aliases
}

// blockOfInstruction returns the index of the block containing an instruction.
func (l *methodLowerer) blockOfInstruction(instructionIndex int) int {
	for index := range l.method.Blocks {
		for _, candidate := range l.method.Blocks[index].Instructions {
			if candidate == instructionIndex {
				return index
			}
		}
	}
	return -1
}

// dominates reports whether every path from the method entry to afterBlock
// passes through beforeBlock. Instructions in the same block dominate in
// program order, which the caller checks separately.
func (l *methodLowerer) dominates(beforeBlock, afterBlock int) bool {
	if beforeBlock == afterBlock || beforeBlock == 0 {
		return true
	}
	seen := make([]bool, len(l.method.Blocks))
	seen[0] = true
	queue := []int{0}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == afterBlock {
			return false
		}
		for _, successor := range l.method.Blocks[current].Successors {
			if successor == beforeBlock || seen[successor] {
				continue
			}
			seen[successor] = true
			queue = append(queue, successor)
		}
	}
	return true
}

func (l *methodLowerer) validateDelegationShape(delegation int, thisRegister uint16) error {
	delegationOffset := l.method.Instructions[delegation].Offset
	l.delegationIndex = delegation
	aliases := map[uint16]bool{thisRegister: true}
	isThis := func(register uint16) bool { return aliases[register] }
	hasBranch := false
	for index := range l.method.Instructions {
		instruction := &l.method.Instructions[index]
		if index < delegation {
			uses, definitions := l.method.EffectsAt(index)
			aliasMove := instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09 && isThis(instruction.Regs[1])
			if isBranchOpcode(instruction.Opcode) {
				// Branches before delegation are argument computation; they may
				// not touch uninitialized this, and deferred stores must still
				// dominate the constructor call.
				for _, register := range uses {
					if isThis(register) {
						return l.uninitializedThisUse(instruction, register)
					}
				}
				hasBranch = true
				continue
			}
			// Only the registers the instruction reads matter; defining an alias
			// register (for example moving a parameter into it) retires the alias.
			usesThis := false
			aliased := uint16(0)
			for _, register := range uses {
				if isThis(register) {
					usesThis = true
					aliased = register
					break
				}
			}
			if usesThis {
				switch {
				case aliasMove:
					// Aliasing uninitialized this is inert; the alias is only
					// meaningful once it is used, which is checked separately.
				case instruction.Opcode >= 0x59 && instruction.Opcode <= 0x5f && isThis(instruction.Regs[1]) && !isThis(instruction.Regs[0]):
					// DEX permits field stores on uninitialized this before
					// super(). The JVM does not, so the value is captured at the
					// original position and the store is emitted after delegation.
					l.deferredStores = append(l.deferredStores, deferredStore{index: index, register: instruction.Regs[0]})
				default:
					return l.uninitializedThisUse(instruction, aliased)
				}
			}
			if aliasMove {
				aliases[instruction.Regs[0]] = true
			} else {
				for _, register := range definitions {
					if register != thisRegister {
						delete(aliases, register)
					}
				}
			}
		}
		if index > delegation && instruction.Target >= 0 && uint32(instruction.Target) <= delegationOffset {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("control flow may repeat constructor delegation"))
		}
	}
	if hasBranch {
		delegationBlock := l.blockOfInstruction(delegation)
		for _, store := range l.deferredStores {
			storeBlock := l.blockOfInstruction(store.index)
			if storeBlock < 0 || delegationBlock < 0 || storeBlock == delegationBlock && store.index > delegation {
				return methodDiagnostic(l.method.Context, &l.method.Instructions[store.index], "", fmt.Errorf("field store before delegation is not on every path to the constructor call"))
			}
			if storeBlock != delegationBlock && !l.dominates(storeBlock, delegationBlock) {
				return methodDiagnostic(l.method.Context, &l.method.Instructions[store.index], "", fmt.Errorf("field store before delegation is not on every path to the constructor call"))
			}
		}
	}
	return nil
}

// isAllocationAlias reports whether the register still aliases an allocation
// of the given class. A direct constructor call on such a register is a second
// initialization site on another path.
func (l *methodLowerer) isAllocationAlias(register uint16, descriptor string) bool {
	for _, allocation := range l.method.Allocations {
		if allocation.Descriptor != descriptor {
			continue
		}
		for _, alias := range allocation.AliasRegisters {
			if alias == register {
				return true
			}
		}
	}
	return false
}

func (l *methodLowerer) uninitializedThisUse(instruction *ir.Instruction, register uint16) error {
	return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this v%d is used before constructor delegation", register))
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
