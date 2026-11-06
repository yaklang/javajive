package ir

import (
	"sort"

	"github.com/yaklang/javajive/dex2jar/model"
)

// instructionConstraints separates the category an instruction reads from its
// operands (uses) from the category it writes into its destination (defs). The
// distinction matters when one register is both: `new-array v1, v1` reads an int
// size and writes an array reference into the same register.
func instructionConstraints(method *Method, file *model.File, index int, current []Category, allocations []Allocation) (uses, defs map[uint16]Category, err error) {
	instruction := method.Instructions[index]
	uses = make(map[uint16]Category)
	defs = make(map[uint16]Category)
	intCategory := Category{Kind: IntLike}
	refCategory := Category{Kind: Reference}
	use := func(register uint16, category Category) { uses[register] = category }
	define := func(register uint16, category Category) { defs[register] = category }

	switch {
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x03 || instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09:
		source := current[instruction.Regs[1]]
		if instruction.Opcode >= 0x07 {
			if source.Kind == Unknown {
				source = refCategory
			}
		} else if source.Kind == Unknown {
			source = current[instruction.Regs[0]]
		}
		define(instruction.Regs[0], source)
		use(instruction.Regs[1], source)
	case instruction.Opcode == 0x0a:
		define(instruction.Regs[0], intCategory)
	case instruction.Opcode == 0x0c:
		define(instruction.Regs[0], refCategory)
	case instruction.Opcode == 0x0d:
		// The handler entry pushes the thrown exception; move-exception stores it.
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/Throwable;"})
	case instruction.Opcode == 0x0f:
		use(instruction.Regs[0], intCategory)
	case instruction.Opcode == 0x11 || instruction.Opcode == 0x27:
		use(instruction.Regs[0], refCategory)
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15:
		if instruction.Literal == 0 {
			define(instruction.Regs[0], Category{Kind: Null})
		} else {
			define(instruction.Regs[0], intCategory)
		}
	case instruction.Opcode == 0x1a || instruction.Opcode == 0x1b:
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/String;"})
	case instruction.Opcode == 0x1c:
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/Class;"})
	case instruction.Opcode == 0x1f:
		use(instruction.Regs[0], refCategory)
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: file.Types[instruction.Index].Descriptor})
	case instruction.Opcode == 0x20 || instruction.Opcode == 0x21:
		use(instruction.Regs[1], refCategory)
		define(instruction.Regs[0], intCategory)
	case instruction.Opcode == 0x22:
		allocation := allocationAt(allocations, index)
		define(instruction.Regs[0], Category{Kind: Uninitialized, Descriptor: allocation.Descriptor, Allocation: allocation.ID})
	case instruction.Opcode == 0x23:
		use(instruction.Regs[1], intCategory)
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: file.Types[instruction.Index].Descriptor})
	case instruction.Opcode == 0x24 || instruction.Opcode == 0x25:
		component := file.Types[instruction.Index].Descriptor[1:]
		category := intCategory
		if len(component) > 0 && (component[0] == 'L' || component[0] == '[') {
			category = refCategory
		}
		for _, register := range instruction.Regs {
			use(register, category)
		}
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x37:
		left, right := current[instruction.Regs[0]], current[instruction.Regs[1]]
		if instruction.Opcode >= 0x34 || left.Kind == IntLike || right.Kind == IntLike {
			use(instruction.Regs[0], intCategory)
			use(instruction.Regs[1], intCategory)
		} else if left.Kind != Unknown {
			use(instruction.Regs[1], left)
		} else if right.Kind != Unknown {
			use(instruction.Regs[0], right)
		}
	case instruction.Opcode >= 0x38 && instruction.Opcode <= 0x3d:
		if instruction.Opcode >= 0x3a {
			use(instruction.Regs[0], intCategory)
		}
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x4a:
		use(instruction.Regs[1], refCategory)
		use(instruction.Regs[2], intCategory)
		if instruction.Opcode == 0x46 {
			define(instruction.Regs[0], refCategory)
		} else {
			define(instruction.Regs[0], intCategory)
		}
	case instruction.Opcode >= 0x4b && instruction.Opcode <= 0x51:
		value := intCategory
		if instruction.Opcode == 0x4d {
			value = refCategory
		}
		use(instruction.Regs[0], value)
		use(instruction.Regs[1], refCategory)
		use(instruction.Regs[2], intCategory)
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x6d:
		field := file.Fields[instruction.Index]
		kind, _ := DescriptorCategory(field.Type)
		category := Category{Kind: kind, Descriptor: field.Type}
		switch {
		case instruction.Opcode <= 0x58:
			use(instruction.Regs[1], refCategory)
			define(instruction.Regs[0], category)
		case instruction.Opcode <= 0x5f:
			use(instruction.Regs[1], refCategory)
			use(instruction.Regs[0], category)
		case instruction.Opcode <= 0x66:
			define(instruction.Regs[0], category)
		default:
			use(instruction.Regs[0], category)
		}
	case isInvoke(instruction.Opcode):
		callee := file.Methods[instruction.Index]
		proto := file.Protos[callee.ProtoIndex]
		register := 0
		if !isStaticInvoke(instruction.Opcode) {
			use(instruction.Regs[0], Category{Kind: Reference, Descriptor: callee.Class})
			register++
		}
		for _, descriptor := range proto.Parameters {
			if register >= len(instruction.Regs) {
				break
			}
			kind, _ := DescriptorCategory(descriptor)
			use(instruction.Regs[register], Category{Kind: kind, Descriptor: descriptor})
			register++
		}
	case instruction.Opcode == 0x7b || instruction.Opcode == 0x7c || instruction.Opcode >= 0x8d && instruction.Opcode <= 0x8f:
		use(instruction.Regs[1], intCategory)
		define(instruction.Regs[0], intCategory)
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0x9a:
		use(instruction.Regs[1], intCategory)
		use(instruction.Regs[2], intCategory)
		define(instruction.Regs[0], intCategory)
	case instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xba:
		use(instruction.Regs[0], intCategory)
		use(instruction.Regs[1], intCategory)
		define(instruction.Regs[0], intCategory)
	case instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		use(instruction.Regs[1], intCategory)
		define(instruction.Regs[0], intCategory)
	}
	return uses, defs, nil
}

func allocationAt(allocations []Allocation, instruction int) Allocation {
	for _, allocation := range allocations {
		if allocation.NewInstruction == instruction {
			return allocation
		}
	}
	return Allocation{}
}

// sortedRegisters returns the keys of a register set in ascending order so
// analysis output is deterministic.
func sortedRegisters(set map[uint16]bool) []uint16 {
	registers := make([]uint16, 0, len(set))
	for register := range set {
		registers = append(registers, register)
	}
	sort.Slice(registers, func(i, j int) bool { return registers[i] < registers[j] })
	return registers
}
