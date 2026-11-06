package ir

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/dex2jar/model"
)

// instructionConstraints separates the category an instruction reads from its
// operands (uses) from the category it writes into its destination (defs). The
// distinction matters when one register is both: `new-array v1, v1` reads an int
// size and writes an array reference into the same register.
func instructionConstraints(method *Method, file *model.File, index int, current []Category, allocations []Allocation, final bool) (uses, defs map[uint16]Category, err error) {
	instruction := method.Instructions[index]
	uses = make(map[uint16]Category)
	defs = make(map[uint16]Category)
	intCategory := Category{Kind: IntLike}
	refCategory := Category{Kind: Reference}
	use := func(register uint16, category Category) { uses[register] = category }
	define := func(register uint16, category Category) { defs[register] = category }

	// The constructor initializes every alias that still points at the new
	// object, so after it runs those registers hold an ordinary reference.
	for _, allocation := range allocations {
		if allocation.Constructor != index {
			continue
		}
		for _, register := range allocation.AliasRegisters {
			define(register, Category{Kind: Reference, Descriptor: allocation.Descriptor})
		}
	}

	switch {
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09:
		source := current[instruction.Regs[1]]
		switch {
		case instruction.Opcode >= 0x07:
			if source.Kind == Unknown {
				source = refCategory
			}
		case instruction.Opcode >= 0x04:
			// move-wide carries a category-2 value; only its interpretation is
			// still open.
			if source.Kind == Unknown {
				source = Category{Kind: Bits64}
			}
		default:
			if source.Kind == Unknown {
				source = current[instruction.Regs[0]]
			}
		}
		define(instruction.Regs[0], source)
		use(instruction.Regs[1], source)
	case instruction.Opcode == 0x0a:
		// A 32-bit result is either an int or a float; the callee decides.
		define(instruction.Regs[0], moveResultCategory(method, file, index, intCategory))
	case instruction.Opcode == 0x0b:
		define(instruction.Regs[0], moveResultCategory(method, file, index, Category{Kind: Bits64}))
	case instruction.Opcode == 0x0c:
		define(instruction.Regs[0], moveResultCategory(method, file, index, refCategory))
	case instruction.Opcode == 0x0d:
		// The handler entry pushes the thrown exception; move-exception stores it.
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/Throwable;"})
	case instruction.Opcode == 0x0f:
		// A 32-bit return is either an int or a float; the prototype decides.
		kind, err := DescriptorCategory(method.Proto.ReturnType)
		if err != nil {
			return nil, nil, err
		}
		use(instruction.Regs[0], Category{Kind: kind, Descriptor: method.Proto.ReturnType})
	case instruction.Opcode == 0x10:
		kind, err := DescriptorCategory(method.Proto.ReturnType)
		if err != nil {
			return nil, nil, err
		}
		use(instruction.Regs[0], Category{Kind: kind, Descriptor: method.Proto.ReturnType})
	case instruction.Opcode == 0x11 || instruction.Opcode == 0x27:
		use(instruction.Regs[0], refCategory)
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15:
		fallback := Category{Kind: Bits32}
		if instruction.Literal == 0 {
			fallback = Category{Kind: Null}
		}
		define(instruction.Regs[0], method.constantCategory(index, fallback))
	case instruction.Opcode >= 0x16 && instruction.Opcode <= 0x19:
		define(instruction.Regs[0], method.constantCategory(index, Category{Kind: Bits64}))
	case instruction.Opcode == 0x1a || instruction.Opcode == 0x1b:
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/String;"})
	case instruction.Opcode == 0x1c:
		define(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/Class;"})
	case instruction.Opcode == 0x1d || instruction.Opcode == 0x1e:
		use(instruction.Regs[0], refCategory)
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
		category, err := DescriptorCategory(component)
		if err != nil {
			return nil, nil, err
		}
		for _, register := range instruction.Regs {
			use(register, Category{Kind: category, Descriptor: component})
		}
	case instruction.Opcode == 0x26:
		array := current[instruction.Regs[0]]
		if len(array.Descriptor) < 2 || array.Descriptor[0] != '[' {
			if !final {
				use(instruction.Regs[0], refCategory)
				return uses, defs, nil
			}
			return nil, nil, fmt.Errorf("fill-array-data register v%d has no array type", instruction.Regs[0])
		}
		width, err := fillArrayElementWidth(array.Descriptor[1:])
		if err != nil {
			return nil, nil, err
		}
		if int(instruction.DataWidth) != width {
			return nil, nil, fmt.Errorf("fill-array-data declares %d-byte elements; %q needs %d", instruction.DataWidth, array.Descriptor[1:], width)
		}
		if width > 0 && len(instruction.Data)%width != 0 {
			return nil, nil, fmt.Errorf("fill-array-data has %d bytes, which is not a whole number of %d-byte elements", len(instruction.Data), width)
		}
		use(instruction.Regs[0], Category{Kind: Reference, Descriptor: array.Descriptor})
	case instruction.Opcode == 0x2b || instruction.Opcode == 0x2c:
		// The switch falls through when no case matches; the register only
		// supplies the selector.
		use(instruction.Regs[0], intCategory)
	case instruction.Opcode >= 0x2d && instruction.Opcode <= 0x31:
		var kind CategoryKind
		switch instruction.Opcode {
		case 0x2d, 0x2e:
			kind = Float
		case 0x2f, 0x30:
			kind = Double
		default:
			kind = Long
		}
		use(instruction.Regs[1], Category{Kind: kind})
		use(instruction.Regs[2], Category{Kind: kind})
		define(instruction.Regs[0], intCategory)
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
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x51:
		// The element type is not encoded in the instruction: DEX tracks it in
		// the array register's type, which the category analysis carries.
		array := current[instruction.Regs[1]]
		if len(array.Descriptor) < 2 || array.Descriptor[0] != '[' {
			if !final {
				// During the category fixpoint a predecessor entry may not be
				// resolved yet; the final walk reports a genuinely untyped array.
				use(instruction.Regs[1], refCategory)
				return uses, defs, nil
			}
			return nil, nil, fmt.Errorf("array register v%d has no array type", instruction.Regs[1])
		}
		component := array.Descriptor[1:]
		if err := validateArrayElementKind(instruction.Opcode, component); err != nil {
			return nil, nil, err
		}
		kind, err := DescriptorCategory(component)
		if err != nil {
			return nil, nil, err
		}
		value := Category{Kind: kind, Descriptor: component}
		use(instruction.Regs[1], Category{Kind: Reference, Descriptor: array.Descriptor})
		use(instruction.Regs[2], intCategory)
		if instruction.Opcode <= 0x4a {
			define(instruction.Regs[0], value)
		} else {
			use(instruction.Regs[0], value)
		}
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x6d:
		field := file.Fields[instruction.Index]
		kind, err := DescriptorCategory(field.Type)
		if err != nil {
			return nil, nil, err
		}
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
		position := 0
		if !isStaticInvoke(instruction.Opcode) {
			use(instruction.Regs[0], Category{Kind: Reference, Descriptor: callee.Class})
			position++
		}
		for _, descriptor := range proto.Parameters {
			if position >= len(instruction.Regs) {
				break
			}
			kind, err := DescriptorCategory(descriptor)
			if err != nil {
				return nil, nil, err
			}
			use(instruction.Regs[position], Category{Kind: kind, Descriptor: descriptor})
			position += kind.Words()
		}
	case instruction.Opcode >= 0x7b && instruction.Opcode <= 0x8f:
		source, destination := unaryCategories(instruction.Opcode)
		use(instruction.Regs[1], Category{Kind: source})
		define(instruction.Regs[0], Category{Kind: destination})
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0xaf:
		kind, shift, ok := binaryCategories(instruction.Opcode)
		if !ok {
			return nil, nil, nil
		}
		use(instruction.Regs[1], Category{Kind: kind})
		if shift {
			use(instruction.Regs[2], intCategory)
		} else {
			use(instruction.Regs[2], Category{Kind: kind})
		}
		define(instruction.Regs[0], Category{Kind: kind})
	case instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xcf:
		kind, shift, ok := binaryTwoAddressCategories(instruction.Opcode)
		if !ok {
			return nil, nil, nil
		}
		use(instruction.Regs[0], Category{Kind: kind})
		if shift {
			use(instruction.Regs[1], intCategory)
		} else {
			use(instruction.Regs[1], Category{Kind: kind})
		}
		define(instruction.Regs[0], Category{Kind: kind})
	case instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		use(instruction.Regs[1], intCategory)
		define(instruction.Regs[0], intCategory)
	}
	return uses, defs, nil
}

// moveResultCategory takes the result category from the preceding invoke. The
// move-result instruction itself carries no type, so the callee prototype is
// the authority on whether the value is an int, a float, a reference, a long or
// a double.
func moveResultCategory(method *Method, file *model.File, index int, fallback Category) Category {
	if index > 0 {
		previous := method.Instructions[index-1]
		if isInvoke(previous.Opcode) && int(previous.Index) < len(file.Methods) {
			callee := file.Methods[previous.Index]
			if int(callee.ProtoIndex) < len(file.Protos) {
				proto := file.Protos[callee.ProtoIndex]
				kind, err := DescriptorCategory(proto.ReturnType)
				if err == nil {
					return Category{Kind: kind, Descriptor: proto.ReturnType}
				}
			}
		}
	}
	return fallback
}

// unaryCategories maps an arithmetic conversion to its source and destination
// categories.
func unaryCategories(opcode byte) (source, destination CategoryKind) {
	switch opcode {
	case 0x7b, 0x7c, 0x8d, 0x8e, 0x8f:
		return IntLike, IntLike
	case 0x7d, 0x7e:
		return Long, Long
	case 0x7f:
		return Float, Float
	case 0x80:
		return Double, Double
	case 0x81:
		return IntLike, Long
	case 0x82:
		return IntLike, Float
	case 0x83:
		return IntLike, Double
	case 0x84:
		return Long, IntLike
	case 0x85:
		return Long, Float
	case 0x86:
		return Long, Double
	case 0x87:
		return Float, IntLike
	case 0x88:
		return Float, Long
	case 0x89:
		return Float, Double
	case 0x8a:
		return Double, IntLike
	case 0x8b:
		return Double, Long
	default: // 0x8c
		return Double, Float
	}
}

// binaryCategories maps a three-register arithmetic opcode to the operand
// category and whether the second source is a shift distance.
func binaryCategories(opcode byte) (kind CategoryKind, shift, ok bool) {
	switch {
	case opcode <= 0x9a:
		kind = IntLike
	case opcode <= 0xa2:
		kind = Long
	case opcode <= 0xa5:
		return Long, true, true
	case opcode <= 0xaa:
		kind = Float
	default:
		kind = Double
	}
	return kind, false, true
}

// binaryTwoAddressCategories maps a /2addr arithmetic opcode to the operand
// category and whether the second source is a shift distance.
func binaryTwoAddressCategories(opcode byte) (kind CategoryKind, shift, ok bool) {
	switch {
	case opcode <= 0xba:
		kind = IntLike
	case opcode <= 0xc2:
		kind = Long
	case opcode <= 0xc5:
		return Long, true, true
	case opcode <= 0xca:
		kind = Float
	default:
		kind = Double
	}
	return kind, false, true
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

// sortedCategoryRegisters returns the keys of a category map in ascending order.
func sortedCategoryRegisters(set map[uint16]Category) []uint16 {
	registers := make([]uint16, 0, len(set))
	for register := range set {
		registers = append(registers, register)
	}
	sort.Slice(registers, func(i, j int) bool { return registers[i] < registers[j] })
	return registers
}
