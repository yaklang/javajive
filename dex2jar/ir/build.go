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
	result.Blocks, err = buildBlocks(instructions, uint32(len(method.Code.Instructions)), ctx)
	if err != nil {
		return nil, err
	}
	result.RegisterTypes, result.Allocations, err = analyzeRegisters(result, in.File)
	if err != nil {
		return nil, err
	}
	populateBlockEntries(result)
	return result, nil
}

func validateInstructions(method *Method, file *model.File) error {
	if len(method.Code.Tries) != 0 {
		return diagnostic(method.Context, nil, "try/catch", fmt.Errorf("try/catch is unsupported"))
	}
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
				return diagnostic(method.Context, instruction, "", fmt.Errorf("unsupported opcode family: wide/float/double"))
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
			if kind == descriptorWideOrFloat {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("wide/floating descriptor %q is unsupported", descriptor))
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
			if instruction.Opcode == 0x0a && kind != IntLike || instruction.Opcode == 0x0c && kind != Reference {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("move-result category does not match invoke result %s", kind))
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
	case opcode == 0x0d:
		return "exception-handler entry"
	case opcode >= 0x1d && opcode <= 0x1e:
		return "monitor"
	case opcode >= 0x24 && opcode <= 0x26:
		return "filled/fill-array"
	case opcode >= 0x2b && opcode <= 0x2c:
		return "switch"
	case opcode >= 0x47 && opcode <= 0x4a || opcode >= 0x4e && opcode <= 0x51:
		return "non-int/non-reference array element"
	case opcode >= 0xfa && opcode <= 0xfd:
		return "polymorphic/custom invocation"
	case opcode >= 0xfe:
		return "post-DEX-035 method constant"
	default:
		return "wide/float/double"
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
		if kind != Int {
			return fmt.Errorf("return does not match return descriptor %q", descriptor)
		}
	case 0x10:
		return fmt.Errorf("unsupported opcode family: wide/float/double")
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
	for _, descriptor := range proto.Parameters {
		if _, err := DescriptorCategory(descriptor); err != nil {
			return 0, err
		}
	}
	return len(proto.Parameters), nil
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

func analyzeRegisters(method *Method, file *model.File) ([]Category, []Allocation, error) {
	registers := make([]Category, method.Code.RegistersSize)
	parameterBase := int(method.Code.RegistersSize - method.Code.InsSize)
	parameter := parameterBase
	if method.AccessFlags&dexAccessStatic == 0 {
		registers[parameter] = Category{Kind: Reference, Descriptor: method.Context.Class}
		parameter++
	}
	for _, descriptor := range method.Proto.Parameters {
		kind, _ := DescriptorCategory(descriptor)
		registers[parameter] = Category{Kind: kind, Descriptor: descriptor}
		parameter++
	}
	allocations := make([]Allocation, 0)
	for index := range method.Instructions {
		instruction := method.Instructions[index]
		if instruction.Opcode == 0x22 {
			allocation, err := validateAllocation(method, file, index)
			if err != nil {
				return nil, nil, err
			}
			allocations = append(allocations, allocation)
		}
	}
	for pass := 0; pass <= len(method.Instructions); pass++ {
		changed := false
		for index := range method.Instructions {
			instruction := method.Instructions[index]
			constraints, err := instructionConstraints(method, file, index, registers, allocations)
			if err != nil {
				return nil, nil, diagnostic(method.Context, &instruction, "", err)
			}
			for register, category := range constraints {
				if category.Kind == Unknown {
					continue
				}
				merged, didChange, err := mergeCategory(registers[register], category)
				if err != nil {
					return nil, nil, diagnostic(method.Context, &instruction, "", fmt.Errorf("v%d: %w", register, err))
				}
				registers[register], changed = merged, changed || didChange
			}
		}
		if !changed {
			break
		}
	}
	return registers, allocations, nil
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

func instructionConstraints(method *Method, file *model.File, index int, current []Category, allocations []Allocation) (map[uint16]Category, error) {
	instruction := method.Instructions[index]
	constraints := make(map[uint16]Category)
	intCategory := Category{Kind: IntLike}
	refCategory := Category{Kind: Reference}
	set := func(register uint16, category Category) { constraints[register] = category }
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
		set(instruction.Regs[0], source)
		set(instruction.Regs[1], source)
	case instruction.Opcode == 0x0a:
		set(instruction.Regs[0], intCategory)
	case instruction.Opcode == 0x0c:
		set(instruction.Regs[0], refCategory)
	case instruction.Opcode == 0x0f:
		set(instruction.Regs[0], intCategory)
	case instruction.Opcode == 0x11 || instruction.Opcode == 0x27:
		set(instruction.Regs[0], refCategory)
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15:
		if instruction.Literal == 0 {
			set(instruction.Regs[0], Category{Kind: Null})
		} else {
			set(instruction.Regs[0], intCategory)
		}
	case instruction.Opcode == 0x1a || instruction.Opcode == 0x1b:
		set(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/String;"})
	case instruction.Opcode == 0x1c:
		set(instruction.Regs[0], Category{Kind: Reference, Descriptor: "Ljava/lang/Class;"})
	case instruction.Opcode == 0x1f:
		set(instruction.Regs[0], Category{Kind: Reference, Descriptor: file.Types[instruction.Index].Descriptor})
	case instruction.Opcode == 0x20:
		set(instruction.Regs[0], intCategory)
		set(instruction.Regs[1], refCategory)
	case instruction.Opcode == 0x21:
		set(instruction.Regs[0], intCategory)
		set(instruction.Regs[1], refCategory)
	case instruction.Opcode == 0x22:
		allocation := allocationAt(allocations, index)
		set(instruction.Regs[0], Category{Kind: Uninitialized, Descriptor: allocation.Descriptor, Allocation: allocation.ID})
	case instruction.Opcode == 0x23:
		set(instruction.Regs[0], Category{Kind: Reference, Descriptor: file.Types[instruction.Index].Descriptor})
		set(instruction.Regs[1], intCategory)
	case instruction.Opcode == 0x24 || instruction.Opcode == 0x25:
		component := file.Types[instruction.Index].Descriptor[1:]
		if len(component) > 0 && (component[0] == 'L' || component[0] == '[') {
			for _, register := range instruction.Regs {
				set(register, refCategory)
			}
		} else {
			for _, register := range instruction.Regs {
				set(register, intCategory)
			}
		}
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x37:
		left, right := current[instruction.Regs[0]], current[instruction.Regs[1]]
		if instruction.Opcode >= 0x34 || left.Kind == IntLike || right.Kind == IntLike {
			set(instruction.Regs[0], intCategory)
			set(instruction.Regs[1], intCategory)
		} else if left.Kind != Unknown {
			set(instruction.Regs[1], left)
		} else if right.Kind != Unknown {
			set(instruction.Regs[0], right)
		}
	case instruction.Opcode >= 0x38 && instruction.Opcode <= 0x3d:
		if instruction.Opcode >= 0x3a {
			set(instruction.Regs[0], intCategory)
		}
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x51:
		set(instruction.Regs[1], refCategory)
		set(instruction.Regs[2], intCategory)
		if instruction.Opcode == 0x46 || instruction.Opcode == 0x4d {
			set(instruction.Regs[0], refCategory)
		} else {
			set(instruction.Regs[0], intCategory)
		}
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x6d:
		field := file.Fields[instruction.Index]
		kind, _ := DescriptorCategory(field.Type)
		category := Category{Kind: kind, Descriptor: field.Type}
		set(instruction.Regs[0], category)
		if instruction.Opcode <= 0x5f {
			set(instruction.Regs[1], refCategory)
		}
	case isInvoke(instruction.Opcode):
		callee := file.Methods[instruction.Index]
		proto := file.Protos[callee.ProtoIndex]
		register := 0
		if !isStaticInvoke(instruction.Opcode) {
			set(instruction.Regs[0], Category{Kind: Reference, Descriptor: callee.Class})
			register++
		}
		for _, descriptor := range proto.Parameters {
			kind, _ := DescriptorCategory(descriptor)
			set(instruction.Regs[register], Category{Kind: kind, Descriptor: descriptor})
			register++
		}
	case instruction.Opcode == 0x7b || instruction.Opcode == 0x7c || instruction.Opcode >= 0x8d && instruction.Opcode <= 0x8f:
		set(instruction.Regs[0], intCategory)
		set(instruction.Regs[1], intCategory)
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0x9a:
		for _, register := range instruction.Regs {
			set(register, intCategory)
		}
	case instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xba:
		for _, register := range instruction.Regs {
			set(register, intCategory)
		}
	case instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		for _, register := range instruction.Regs {
			set(register, intCategory)
		}
	}
	return constraints, nil
}

func allocationAt(allocations []Allocation, instruction int) Allocation {
	for _, allocation := range allocations {
		if allocation.NewInstruction == instruction {
			return allocation
		}
	}
	return Allocation{}
}

func mergeCategory(left, right Category) (Category, bool, error) {
	if right.Kind == Unknown {
		return left, false, nil
	}
	if left.Kind == Conflict || right.Kind == Conflict {
		return Category{Kind: Conflict}, left.Kind != Conflict, fmt.Errorf("conflicting register categories")
	}
	if left.Kind == Unknown || left.Kind == Null {
		return right, true, nil
	}
	if right.Kind == Null {
		return left, false, nil
	}
	leftReference := left.Kind == Reference || left.Kind == Uninitialized
	rightReference := right.Kind == Reference || right.Kind == Uninitialized
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

func populateBlockEntries(method *Method) {
	for index := range method.Blocks {
		method.Blocks[index].Entry = append([]Category(nil), method.RegisterTypes...)
	}
}
