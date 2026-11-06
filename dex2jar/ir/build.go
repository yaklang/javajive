package ir

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/javajive/dex2jar/model"
)

const dexAccessStatic = 0x0008

// Build decodes, validates, builds the CFG, and derives stable register homes.
func Build(in Input) (*Method, error) {
	ctx := contextFor(in)
	if in.File == nil || in.Class == nil || in.Method == nil || in.Method.Code == nil {
		return nil, diagnostic(ctx, nil, "method", fmt.Errorf("incomplete method input"))
	}
	if err := validateFileMetadata(in.File); err != nil {
		return nil, diagnostic(ctx, nil, "metadata", err)
	}
	if err := validateInputMetadata(in); err != nil {
		return nil, diagnostic(ctx, nil, "metadata", err)
	}
	method := in.Method
	if int(method.Method.ProtoIndex) >= len(in.File.Protos) {
		return nil, diagnostic(ctx, nil, "method", fmt.Errorf("proto index %d is out of range", method.Method.ProtoIndex))
	}
	proto := in.File.Protos[method.Method.ProtoIndex]
	instructions, err := Decode(method.Code, ctx)
	if err != nil {
		return nil, err
	}
	result := &Method{Code: method.Code, Instructions: instructions, Context: ctx, Proto: proto, AccessFlags: method.AccessFlags}
	if err := validateInstructions(result, in.File); err != nil {
		return nil, err
	}
	sites, err := handlerSites(method.Code, instructions, uint32(len(method.Code.Instructions)), ctx)
	if err != nil {
		return nil, err
	}
	result.Blocks, err = buildBlocks(instructions, uint32(len(method.Code.Instructions)), ctx, sites)
	if err != nil {
		return nil, err
	}
	if err := validateExceptionEntries(result); err != nil {
		return nil, err
	}
	allocations, err := collectAllocations(result, in.File)
	if err != nil {
		return nil, err
	}
	result.Allocations = allocations
	result.RegisterTypes, err = analyzeCategories(result, in.File, allocations)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateInstructions(method *Method, file *model.File) error {
	registerCount := uint32(method.Code.RegistersSize)
	for index := range method.Instructions {
		instruction := &method.Instructions[index]
		info := opcodeTable[instruction.Opcode]
		if !info.supported {
			return diagnostic(method.Context, instruction, "", fmt.Errorf("construct is outside the supported DEX 035 subset; unsupported opcode family: %s", unsupportedFamily(instruction.Opcode)))
		}
		for _, register := range instruction.Regs {
			if uint32(register) >= registerCount {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("register v%d is outside register file v0..v%d", register, registerCount-1))
			}
		}
		if instruction.Opcode == 0x1a || instruction.Opcode == 0x1b {
			if instruction.Index >= uint32(len(file.Strings)) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("string index %d is out of range", instruction.Index))
			}
		}
		if instruction.Opcode == 0x24 || instruction.Opcode == 0x25 {
			if instruction.Index >= uint32(len(file.Types)) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("type index %d is out of range", instruction.Index))
			}
			descriptor := file.Types[instruction.Index].Descriptor
			if len(descriptor) < 2 || descriptor[0] != '[' {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("filled-new-array type %q is not an array", descriptor))
			}
			if component := descriptor[1:]; component == "J" || component == "D" || component == "F" {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("filled-new-array of a wide or floating element is unsupported"))
			}
		}
		if instruction.Opcode == 0x1c || instruction.Opcode == 0x1f || instruction.Opcode == 0x20 || instruction.Opcode == 0x22 || instruction.Opcode == 0x23 {
			if instruction.Index >= uint32(len(file.Types)) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("type index %d is out of range", instruction.Index))
			}
			descriptor := file.Types[instruction.Index].Descriptor
			kind, err := parseDescriptor(descriptor, instruction.Opcode == 0x1c)
			if err != nil {
				return diagnostic(method.Context, instruction, "", err)
			}
			switch instruction.Opcode {
			case 0x1f, 0x20:
				if kind != descriptorReference {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("type %q is not a reference type", descriptor))
				}
			case 0x22:
				if kind != descriptorReference || descriptor[0] != 'L' {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("new-instance type %q is not an object type", descriptor))
				}
			case 0x23:
				if kind != descriptorReference || descriptor[0] != '[' {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("new-array type %q is not an array type", descriptor))
				}
			}
		}
		if instruction.Opcode >= 0x52 && instruction.Opcode <= 0x6d {
			if instruction.Index >= uint32(len(file.Fields)) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("field index %d is out of range", instruction.Index))
			}
			if _, err := DescriptorCategory(file.Fields[instruction.Index].Type); err != nil {
				return diagnostic(method.Context, instruction, "", err)
			}
		}
		if isInvoke(instruction.Opcode) {
			if instruction.Index >= uint32(len(file.Methods)) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("method index %d is out of range", instruction.Index))
			}
			callee := file.Methods[instruction.Index]
			if int(callee.ProtoIndex) >= len(file.Protos) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("callee proto index %d is out of range", callee.ProtoIndex))
			}
			proto := file.Protos[callee.ProtoIndex]
			wordCount, err := supportedParameterWords(proto)
			if err != nil {
				return diagnostic(method.Context, instruction, "", err)
			}
			if !isStaticInvoke(instruction.Opcode) {
				wordCount++
			}
			if len(instruction.Regs) != wordCount {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("invoke has %d register words; descriptor requires %d", len(instruction.Regs), wordCount))
			}
			if wordCount > int(method.Code.OutsSize) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("invoke requires %d outgoing register words; outs_size is %d", wordCount, method.Code.OutsSize))
			}
			if _, err := DescriptorCategory(proto.ReturnType); err != nil {
				return diagnostic(method.Context, instruction, "", err)
			}
			if isRangeInvoke(instruction.Opcode) && !contiguous(instruction.Regs) {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("range registers are not contiguous"))
			}
			// A category-2 argument lists both of its registers; they must be
			// the register and its successor.
			position := 0
			if !isStaticInvoke(instruction.Opcode) {
				position++
			}
			for _, descriptor := range proto.Parameters {
				kind, err := DescriptorCategory(descriptor)
				if err != nil {
					return diagnostic(method.Context, instruction, "", err)
				}
				if position+kind.Words() > len(instruction.Regs) {
					break
				}
				if kind.Words() == 2 && instruction.Regs[position+1] != instruction.Regs[position]+1 {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("wide argument v%d must be followed by v%d", instruction.Regs[position], instruction.Regs[position]+1))
				}
				position += kind.Words()
			}
		}
		if isMoveResult(instruction.Opcode) {
			if index == 0 {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result must immediately follow an invoke or filled-new-array"))
			}
			previous := method.Instructions[index-1]
			if !isInvoke(previous.Opcode) && previous.Opcode != 0x24 && previous.Opcode != 0x25 {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result must immediately follow an invoke or filled-new-array"))
			}
			if previous.Opcode == 0x24 || previous.Opcode == 0x25 {
				if instruction.Opcode != 0x0c {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("filled-new-array result requires move-result-object"))
				}
				continue
			}
			kind, err := invokeResultCategory(previous, file)
			if err != nil {
				return diagnostic(method.Context, instruction, "", err)
			}
			if kind == Unknown {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result follows a void invoke"))
			}
			switch instruction.Opcode {
			case 0x0a:
				// A 32-bit result is either an int or a float.
				if kind != IntLike && kind != Float {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result category does not match invoke result %s", kind))
				}
			case 0x0b:
				if kind != Long && kind != Double {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result-wide category does not match invoke result %s", kind))
				}
			case 0x0c:
				if kind != Reference {
					return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result-object category does not match invoke result %s", kind))
				}
			}
		}
		if instruction.Opcode >= 0x0e && instruction.Opcode <= 0x11 {
			if err := validateReturnOpcode(method.Proto.ReturnType, instruction.Opcode); err != nil {
				return diagnostic(method.Context, instruction, "", err)
			}
		}
		if isInvoke(instruction.Opcode) && index+1 < len(method.Instructions) && isMoveResult(method.Instructions[index+1].Opcode) {
			continue
		}
	}
	return validateMethodShape(method)
}

func unsupportedFamily(opcode byte) string {
	switch {
	case opcode >= 0x1d && opcode <= 0x1e:
		return "monitor"
	case opcode == 0x26:
		return "fill-array-data"
	case opcode >= 0x2b && opcode <= 0x2c:
		return "switch"
	case opcode >= 0xfa && opcode <= 0xfd:
		return "polymorphic/custom invocation"
	case opcode >= 0xfe:
		return "post-DEX-035 method constant"
	default:
		return "unimplemented opcode"
	}
}

func validateReturnOpcode(descriptor string, opcode byte) error {
	kind, err := DescriptorCategory(descriptor)
	if err != nil {
		return err
	}
	switch opcode {
	case 0x0e:
		if descriptor != "V" {
			return fmt.Errorf("return-void does not match return descriptor %q", descriptor)
		}
	case 0x0f:
		// A 32-bit return carries either an int or a float.
		if kind != Int && kind != Float {
			return fmt.Errorf("return does not match return descriptor %q", descriptor)
		}
	case 0x10:
		if kind != Long && kind != Double {
			return fmt.Errorf("return-wide does not match return descriptor %q", descriptor)
		}
	case 0x11:
		if kind != Reference {
			return fmt.Errorf("return-object does not match return descriptor %q", descriptor)
		}
	}
	return nil
}

func validateMethodShape(method *Method) error {
	parameters, err := supportedParameterWords(method.Proto)
	if err != nil {
		return diagnostic(method.Context, nil, "method", err)
	}
	if method.AccessFlags&dexAccessStatic == 0 {
		parameters++
	}
	if parameters != int(method.Code.InsSize) {
		return diagnostic(method.Context, nil, "method", fmt.Errorf("ins_size %d does not match descriptor word count %d", method.Code.InsSize, parameters))
	}
	if method.Code.InsSize > method.Code.RegistersSize {
		return diagnostic(method.Context, nil, "method", fmt.Errorf("ins_size %d exceeds registers_size %d", method.Code.InsSize, method.Code.RegistersSize))
	}
	return nil
}

func supportedParameterWords(proto model.Proto) (int, error) {
	if err := validateProto(proto); err != nil {
		return 0, err
	}
	words := 0
	for _, descriptor := range proto.Parameters {
		kind, err := DescriptorCategory(descriptor)
		if err != nil {
			return 0, err
		}
		words += kind.Words()
	}
	return words, nil
}

func isInvoke(opcode byte) bool {
	return opcode >= 0x6e && opcode <= 0x72 || opcode >= 0x74 && opcode <= 0x78
}
func isRangeInvoke(opcode byte) bool  { return opcode >= 0x74 && opcode <= 0x78 }
func isStaticInvoke(opcode byte) bool { return opcode == 0x71 || opcode == 0x77 }
func isDirectInvoke(opcode byte) bool { return opcode == 0x70 || opcode == 0x76 }
func isMoveResult(opcode byte) bool   { return opcode == 0x0a || opcode == 0x0b || opcode == 0x0c }

func contiguous(registers []uint16) bool {
	for index := 1; index < len(registers); index++ {
		if registers[index] != registers[index-1]+1 {
			return false
		}
	}
	return true
}

func invokeResultCategory(instruction Instruction, file *model.File) (CategoryKind, error) {
	callee := file.Methods[instruction.Index]
	return DescriptorCategory(file.Protos[callee.ProtoIndex].ReturnType)
}

// validateExceptionEntries requires move-exception to open a handler block and
// every handler block to start with one.
func validateExceptionEntries(method *Method) error {
	handlerStarts := make(map[uint32]bool)
	for index := range method.Blocks {
		if method.Blocks[index].Handler {
			handlerStarts[method.Blocks[index].Start] = true
		}
	}
	for index := range method.Instructions {
		instruction := &method.Instructions[index]
		if instruction.Opcode != 0x0d {
			continue
		}
		if !handlerStarts[instruction.Offset] {
			return diagnostic(method.Context, instruction, "", fmt.Errorf("move-exception is only valid as the first instruction of an exception handler"))
		}
	}
	for blockIndex := range method.Blocks {
		block := &method.Blocks[blockIndex]
		if !block.Handler {
			continue
		}
		first := method.Instructions[block.Instructions[0]]
		if first.Opcode != 0x0d {
			return diagnostic(method.Context, &first, "", fmt.Errorf("exception handler at offset %d does not start with move-exception", block.Start))
		}
	}
	return nil
}

// collectAllocations validates and indexes every new-instance/constructor pair.
func collectAllocations(method *Method, file *model.File) ([]Allocation, error) {
	allocations := make([]Allocation, 0)
	for index := range method.Instructions {
		if method.Instructions[index].Opcode != 0x22 {
			continue
		}
		allocation, err := validateAllocation(method, file, index)
		if err != nil {
			return nil, err
		}
		allocations = append(allocations, allocation)
	}
	return allocations, nil
}

func validateAllocation(method *Method, file *model.File, newIndex int) (Allocation, error) {
	newInstruction := method.Instructions[newIndex]
	descriptor := file.Types[newInstruction.Index].Descriptor
	if !strings.HasPrefix(descriptor, "L") {
		return Allocation{}, diagnostic(method.Context, &newInstruction, "", fmt.Errorf("new-instance type %q is not an object", descriptor))
	}
	aliases := map[uint16]bool{newInstruction.Regs[0]: true}
	constructor := -1
	interleaved := false
	for index := newIndex + 1; index < len(method.Instructions); index++ {
		instruction := method.Instructions[index]
		if instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09 && aliases[instruction.Regs[1]] {
			aliases[instruction.Regs[0]] = true
			continue
		}
		if isDirectInvoke(instruction.Opcode) && len(instruction.Regs) > 0 && aliases[instruction.Regs[0]] {
			callee := file.Methods[instruction.Index]
			if callee.Name != "<init>" || callee.Class != descriptor {
				return Allocation{}, diagnostic(method.Context, &instruction, "", fmt.Errorf("allocation must be initialized by %s.<init>", descriptor))
			}
			constructor = index
			break
		}
		// Any other instruction may sit between allocation and initialization as
		// long as it never touches the uninitialized value: d8 interleaves
		// argument computation here.
		interleaved = true
		for _, register := range instruction.Regs {
			if aliases[register] {
				return Allocation{}, diagnostic(method.Context, &instruction, "", fmt.Errorf("unsafe uninitialized allocation use or escape before constructor"))
			}
		}
	}
	if constructor < 0 {
		return Allocation{}, diagnostic(method.Context, &newInstruction, "", fmt.Errorf("new-instance has no adjacent safe constructor invocation"))
	}
	aliasRegisters := make([]uint16, 0, len(aliases))
	for register := range aliases {
		aliasRegisters = append(aliasRegisters, register)
	}
	sort.Slice(aliasRegisters, func(i, j int) bool { return aliasRegisters[i] < aliasRegisters[j] })
	return Allocation{
		ID: newInstruction.Offset + 1, Descriptor: descriptor,
		NewInstruction: newIndex, Constructor: constructor,
		AliasRegisters: aliasRegisters, Fused: !interleaved,
	}, nil
}

func mergeCategory(left, right Category) (Category, bool, error) {
	if right.Kind == Unknown {
		return left, false, nil
	}
	if left.Kind == Conflict || right.Kind == Conflict {
		return Category{Kind: Conflict}, left.Kind != Conflict, fmt.Errorf("conflicting register categories")
	}
	if left.Kind == Unknown {
		return right, true, nil
	}
	if left.Kind.isUntyped() || right.Kind.isUntyped() {
		return mergeUntyped(left, right)
	}
	leftReference := left.Kind == Reference || left.Kind == Uninitialized
	rightReference := right.Kind == Reference || right.Kind == Uninitialized
	if !leftReference && !rightReference && left.Kind != right.Kind {
		return Category{Kind: Conflict}, true, fmt.Errorf("incompatible stable local categories %s and %s", left.Kind, right.Kind)
	}
	if leftReference != rightReference {
		return Category{Kind: Conflict}, true, fmt.Errorf("incompatible stable local categories %s and %s", left.Kind, right.Kind)
	}
	if left.Kind == Uninitialized && right.Kind == Uninitialized && left.Allocation != right.Allocation {
		return Category{Kind: Conflict}, true, fmt.Errorf("different allocation identities merge")
	}
	if left.Descriptor == "" && right.Descriptor != "" {
		left.Descriptor = right.Descriptor
		return left, true, nil
	}
	return left, false, nil
}

// mergeUntyped joins a constant whose interpretation is still open with the
// category demanded at a use or at a control-flow join.
func mergeUntyped(left, right Category) (Category, bool, error) {
	untyped, typed := left, right
	if !left.Kind.isUntyped() {
		untyped, typed = right, left
	}
	if typed.Kind.isUntyped() {
		switch {
		case untyped.Kind == typed.Kind:
			return left, false, nil
		case untyped.Kind == Null:
			// A zero constant is a valid 32-bit constant but never supplies the
			// two registers a category-2 value needs.
			if typed.Kind == Bits32 {
				return Category{Kind: Bits32}, true, nil
			}
		case typed.Kind == Null:
			if untyped.Kind == Bits32 {
				return Category{Kind: Bits32}, true, nil
			}
		}
		return Category{Kind: Conflict}, true, fmt.Errorf("incompatible constant categories %s and %s", left.Kind, right.Kind)
	}
	// A null constant has no intrinsic type and adopts the type of its first
	// use. The DEX "zero" value is assignable to int, float and reference, but
	// never supplies the two registers a category-2 value needs.
	switch untyped.Kind {
	case Null:
		if typed.Kind.Words() == 2 {
			return Category{Kind: Conflict}, true, fmt.Errorf("null constant cannot become %s", typed.Kind)
		}
		if typed.Kind == Int || typed.Kind == Float || typed.Kind == Reference {
			return typed, true, nil
		}
	case Bits32:
		if typed.Kind == Int || typed.Kind == Float {
			return typed, true, nil
		}
	case Bits64:
		if typed.Kind == Long || typed.Kind == Double {
			return typed, true, nil
		}
	}
	return Category{Kind: Conflict}, true, fmt.Errorf("constant category %s cannot become %s", untyped.Kind, typed.Kind)
}
