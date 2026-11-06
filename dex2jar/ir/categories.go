package ir

import (
	"fmt"

	"github.com/yaklang/javajive/dex2jar/model"
)

// Definition is the category one instruction writes into one register.
type Definition struct {
	Register int32 // -1 when the instruction defines nothing
	Category Category
}

// EffectsAt reports the registers an instruction reads and writes. Allocation
// effects are semantic: new-instance defines its receiver and the constructor
// call consumes the receiver plus its arguments, regardless of how lowering
// later emits them.
func (m *Method) EffectsAt(index int) (uses, defs []uint16) {
	if m == nil || index < 0 || index >= len(m.Instructions) {
		return nil, nil
	}
	instruction := m.Instructions[index]
	for _, allocation := range m.Allocations {
		if allocation.NewInstruction == index {
			return nil, []uint16{instruction.Regs[0]}
		}
		if allocation.Constructor == index {
			return append([]uint16(nil), instruction.Regs...), nil
		}
	}
	return instructionEffects(instruction)
}

// instructionEffects is the per-opcode use/define table shared by liveness,
// definite-assignment and category analysis.
func instructionEffects(instruction Instruction) (uses, defs []uint16) {
	switch {
	case instruction.Opcode == 0x00 || instruction.Opcode == 0x0e || instruction.Opcode >= 0x28 && instruction.Opcode <= 0x2a:
		return nil, nil
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x03 || instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x0a || instruction.Opcode == 0x0c || instruction.Opcode == 0x0d:
		// move-result and move-exception store an already-produced value.
		return nil, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x0f || instruction.Opcode == 0x11 || instruction.Opcode == 0x27:
		return []uint16{instruction.Regs[0]}, nil
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x1c || instruction.Opcode == 0x22:
		return nil, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x1f:
		return []uint16{instruction.Regs[0]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x20 || instruction.Opcode == 0x21 || instruction.Opcode == 0x23:
		return []uint16{instruction.Regs[1]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode == 0x24 || instruction.Opcode == 0x25:
		return append([]uint16(nil), instruction.Regs...), nil
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x37:
		return append([]uint16(nil), instruction.Regs...), nil
	case instruction.Opcode >= 0x38 && instruction.Opcode <= 0x3d:
		return []uint16{instruction.Regs[0]}, nil
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x4a:
		return []uint16{instruction.Regs[1], instruction.Regs[2]}, []uint16{instruction.Regs[0]}
	case instruction.Opcode >= 0x4b && instruction.Opcode <= 0x51:
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

// CategoryAt returns the value category of register immediately before the
// indexed instruction executes. A zero Category means "not yet known".
func (m *Method) CategoryAt(index int, register uint16) Category {
	if m == nil || m.CategoryStride <= 0 || index < 0 || index >= len(m.Instructions) {
		return Category{}
	}
	if int(register) >= m.CategoryStride {
		return Category{}
	}
	offset := index*m.CategoryStride + int(register)
	if offset < 0 || offset >= len(m.Categories) {
		return Category{}
	}
	return m.Categories[offset]
}

// DefinitionAt returns the category instruction index writes, if any.
func (m *Method) DefinitionAt(index int) (Definition, bool) {
	if m == nil || index < 0 || index >= len(m.Definitions) {
		return Definition{}, false
	}
	definition := m.Definitions[index]
	if definition.Register < 0 {
		return Definition{}, false
	}
	return definition, true
}

// blockRun is the local result of walking one basic block.
type blockRun struct {
	states      [][]Category
	definitions []Definition
	exit        []Category
}

// walkBlock applies the instruction category constraints in program order.
// Definitions replace the previous lifetime of a register; uses must agree with
// the category already established for that register.
func walkBlock(method *Method, file *model.File, allocations []Allocation, blockIndex int, entryState []Category) (*blockRun, error) {
	block := method.Blocks[blockIndex]
	run := &blockRun{
		states:      make([][]Category, 0, len(block.Instructions)),
		definitions: make([]Definition, 0, len(block.Instructions)),
	}
	state := append([]Category(nil), entryState...)
	// A null constant has no intrinsic type; it adopts the type of its first
	// use. Track where the current null lifetime started so the definition and
	// every intermediate state can be refined retroactively.
	nullLifetime := make(map[uint16]int)
	for _, instructionIndex := range block.Instructions {
		run.states = append(run.states, append([]Category(nil), state...))
		position := len(run.states) - 1
		uses, defs, err := instructionConstraints(method, file, instructionIndex, state, allocations)
		if err != nil {
			instruction := &method.Instructions[instructionIndex]
			return nil, diagnostic(method.Context, instruction, "", err)
		}
		// Operand requirements first: they constrain the values read at this
		// point, including a destination register that is also an operand.
		for register, category := range uses {
			if category.Kind == Unknown {
				continue
			}
			merged, _, err := mergeCategory(state[register], category)
			if err != nil {
				instruction := &method.Instructions[instructionIndex]
				return nil, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d: %w", register, err))
			}
			if state[register].Kind == Null && merged.Kind != Null {
				if start, ok := nullLifetime[register]; ok && start < position {
					for index := start + 1; index <= position; index++ {
						run.states[index][register] = merged
					}
					run.definitions[start].Category = merged
					delete(nullLifetime, register)
				}
			}
			state[register] = merged
		}
		definition := Definition{Register: -1}
		for register, category := range defs {
			if category.Kind == Unknown {
				continue
			}
			state[register] = category
			definition = Definition{Register: int32(register), Category: category}
			if category.Kind == Null {
				nullLifetime[register] = position
			} else {
				delete(nullLifetime, register)
			}
		}
		run.definitions = append(run.definitions, definition)
	}
	run.exit = state
	return run, nil
}

// analyzeCategories computes the value category of every register at every
// instruction. Unlike one category per register, this follows register lifetime
// reuse: a DEX register may hold an int in one part of a method and a reference
// in another, exactly like a JVM local.
func analyzeCategories(method *Method, file *model.File, allocations []Allocation) ([]Category, error) {
	registerCount := int(method.Code.RegistersSize)
	entry := make([]Category, registerCount)
	parameter := int(method.Code.RegistersSize - method.Code.InsSize)
	if method.AccessFlags&dexAccessStatic == 0 {
		if parameter < registerCount {
			entry[parameter] = Category{Kind: Reference, Descriptor: method.Context.Class}
		}
		parameter++
	}
	for _, descriptor := range method.Proto.Parameters {
		if parameter >= registerCount {
			break
		}
		kind, _ := DescriptorCategory(descriptor)
		entry[parameter] = Category{Kind: kind, Descriptor: descriptor}
		parameter++
	}

	// usedBeforeDef drives use-site verification; the live sets decide which
	// categories must flow across block boundaries. A register that is live-in
	// but neither used nor defined in a block still has to carry its category to
	// the successors, otherwise the value would arrive untyped.
	usedBeforeDef := make([]map[uint16]bool, len(method.Blocks))
	definedIn := make([]map[uint16]bool, len(method.Blocks))
	for index := range usedBeforeDef {
		usedBeforeDef[index] = make(map[uint16]bool)
		definedIn[index] = make(map[uint16]bool)
	}
	for blockIndex, block := range method.Blocks {
		for _, instructionIndex := range block.Instructions {
			uses, defs := method.EffectsAt(instructionIndex)
			for _, register := range uses {
				if !definedIn[blockIndex][register] {
					usedBeforeDef[blockIndex][register] = true
				}
			}
			for _, register := range defs {
				definedIn[blockIndex][register] = true
			}
		}
	}
	liveIn, liveOut := computeLiveness(method, usedBeforeDef, definedIn)
	// propagate holds the registers whose category must be merged at the join:
	// everything read before a local definition plus everything that stays live
	// across the block without being redefined.
	propagate := make([]map[uint16]bool, len(method.Blocks))
	for index := range propagate {
		propagate[index] = make(map[uint16]bool)
		for register := range usedBeforeDef[index] {
			propagate[index][register] = true
		}
		for register := range liveIn[index] {
			if !definedIn[index][register] {
				propagate[index][register] = true
			}
		}
	}
	_ = liveOut

	entries := make([][]Category, len(method.Blocks))
	for index := range entries {
		entries[index] = make([]Category, registerCount)
	}
	if len(method.Blocks) > 0 {
		copy(entries[0], entry)
	}
	for pass := 0; pass <= len(method.Blocks)+1; pass++ {
		changed := false
		for blockIndex := 1; blockIndex < len(method.Blocks); blockIndex++ {
			next := append([]Category(nil), entries[blockIndex]...)
			for _, register := range sortedRegisters(propagate[blockIndex]) {
				merged := Category{}
				for _, predecessor := range method.Blocks[blockIndex].Predecessors {
					predecessorRun, err := walkBlock(method, file, allocations, predecessor, entries[predecessor])
					if err != nil {
						return nil, err
					}
					incoming := predecessorRun.exit[register]
					if incoming.Kind == Unknown {
						continue
					}
					result, _, err := mergeCategory(merged, incoming)
					if err != nil {
						instruction := &method.Instructions[firstInstruction(method, blockIndex)]
						return nil, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d has incompatible categories at a control-flow join: %w", register, err))
					}
					merged = result
				}
				next[register] = merged
			}
			if !sameCategories(entries[blockIndex], next) {
				entries[blockIndex] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Materialize per-instruction states and definitions.
	stride := registerCount
	method.CategoryStride = stride
	method.Categories = make([]Category, len(method.Instructions)*stride)
	method.Definitions = make([]Definition, len(method.Instructions))
	for index := range method.Definitions {
		method.Definitions[index].Register = -1
	}
	for blockIndex, block := range method.Blocks {
		run, err := walkBlock(method, file, allocations, blockIndex, entries[blockIndex])
		if err != nil {
			return nil, err
		}
		entryState := append([]Category(nil), entries[blockIndex]...)
		if len(block.Entry) != len(entryState) {
			method.Blocks[blockIndex].Entry = entryState
		} else {
			copy(method.Blocks[blockIndex].Entry, entryState)
		}
		for offset, instructionIndex := range block.Instructions {
			copy(method.Categories[instructionIndex*stride:], run.states[offset])
			method.Definitions[instructionIndex] = run.definitions[offset]
		}
	}
	return entry, nil
}

// computeLiveness runs the standard backward live-variable analysis over the
// block graph using the per-instruction use/define table.
func computeLiveness(method *Method, usedBeforeDef, definedIn []map[uint16]bool) ([]map[uint16]bool, []map[uint16]bool) {
	liveIn := make([]map[uint16]bool, len(method.Blocks))
	liveOut := make([]map[uint16]bool, len(method.Blocks))
	for index := range liveIn {
		liveIn[index] = make(map[uint16]bool)
		liveOut[index] = make(map[uint16]bool)
	}
	for pass := 0; pass <= len(method.Blocks)+1; pass++ {
		changed := false
		for blockIndex := len(method.Blocks) - 1; blockIndex >= 0; blockIndex-- {
			out := make(map[uint16]bool)
			for _, successor := range method.Blocks[blockIndex].Successors {
				for register := range liveIn[successor] {
					out[register] = true
				}
			}
			in := make(map[uint16]bool)
			for register := range usedBeforeDef[blockIndex] {
				in[register] = true
			}
			for register := range out {
				if !definedIn[blockIndex][register] {
					in[register] = true
				}
			}
			if !sameRegisterSet(liveOut[blockIndex], out) || !sameRegisterSet(liveIn[blockIndex], in) {
				changed = true
				liveOut[blockIndex], liveIn[blockIndex] = out, in
			}
		}
		if !changed {
			break
		}
	}
	return liveIn, liveOut
}

func sameRegisterSet(left, right map[uint16]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for register := range left {
		if !right[register] {
			return false
		}
	}
	return true
}

func firstInstruction(method *Method, blockIndex int) int {
	if len(method.Blocks[blockIndex].Instructions) == 0 {
		return 0
	}
	return method.Blocks[blockIndex].Instructions[0]
}

func sameCategories(left, right []Category) bool {
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
