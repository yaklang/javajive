package ir

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
)

const dexAccessStatic = 0x0008

// maxRegisterStateCells bounds the dense per-instruction register category
// table and the temporary states used to derive it. Four million cells leave
// ample room for ordinary methods while preventing hostile code/register
// dimensions from amplifying a small code item into an unbounded allocation.
const maxRegisterStateCells = 4 << 20

// Build decodes, validates, builds the CFG, and derives stable register homes.
func Build(in Input) (*Method, error) {
	ctx := contextFor(in)
	if in.File == nil || in.Class == nil || in.Method == nil || in.Method.Code == nil {
		return nil, diagnostic(ctx, nil, "method", fmt.Errorf("incomplete method input"))
	}
	if !in.SkipFileValidation {
		if err := validateFileMetadata(in.File); err != nil {
			return nil, diagnostic(ctx, nil, "metadata", err)
		}
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
	stateCells := uint64(len(instructions)) * uint64(method.Code.RegistersSize)
	if stateCells > maxRegisterStateCells {
		return nil, diagnostic(ctx, nil, "resources", fmt.Errorf("instruction/register state space of %d instructions x %d registers (%d cells) exceeds limit %d", len(instructions), method.Code.RegistersSize, stateCells, maxRegisterStateCells))
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
	result.blockByInstruction = make([]int, len(result.Instructions))
	for blockIndex := range result.Blocks {
		for _, instructionIndex := range result.Blocks[blockIndex].Instructions {
			result.blockByInstruction[instructionIndex] = blockIndex
		}
	}
	result.exceptionTypes = exceptionTypes(method.Code)
	if err := validateExceptionEntries(result); err != nil {
		return nil, err
	}
	allocations, err := candidateAllocations(result, in.File)
	if err != nil {
		return nil, err
	}
	result.Allocations = allocations
	for pass := 0; pass <= len(result.Instructions)+2; pass++ {
		result.indexAllocations()
		result.RegisterTypes, err = analyzeCategories(result, in.File, result.Allocations)
		if err != nil {
			return nil, err
		}
		changed, err := resolveAllocationSites(result, in.File)
		if err != nil {
			return nil, err
		}
		if !changed {
			if err := validateAllocationSites(result, in.File); err != nil {
				return nil, err
			}
			return result, nil
		}
	}
	return nil, diagnostic(result.Context, nil, "allocations", fmt.Errorf("allocation site inference did not converge"))
}

func validateInstructions(method *Method, file *model.File) error {
	registerCount := uint32(method.Code.RegistersSize)
	for index := range method.Instructions {
		instruction := &method.Instructions[index]
		info := opcodeTable[instruction.Opcode]
		if !info.supported {
			family := unsupportedFamily(instruction.Opcode)
			return diagnostic(method.Context, instruction, "", report.Unsupported(family, fmt.Errorf("construct is outside the supported DEX 035 subset; unsupported opcode family: %s", family)))
		}
		for _, register := range instruction.Regs {
			if uint32(register) >= registerCount {
				return diagnostic(method.Context, instruction, "", fmt.Errorf("register v%d is outside register file of %d registers", register, registerCount))
			}
		}
		if wideDefinitionOpcode(instruction.Opcode) && len(instruction.Regs) > 0 && uint32(instruction.Regs[0])+1 >= registerCount {
			return diagnostic(method.Context, instruction, "", fmt.Errorf("wide destination v%d requires register v%d within register file of %d registers", instruction.Regs[0], instruction.Regs[0]+1, registerCount))
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
				return diagnostic(method.Context, instruction, "", report.Unsupported("filled-new-array wide or floating element", fmt.Errorf("filled-new-array of a wide or floating element is unsupported")))
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

// validateExceptionEntries requires move-exception to open a handler block. A
// handler that never reads the caught exception does not begin with one: DEX
// only binds the exception through move-exception, so such a handler simply
// discards it.
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
	return nil
}

// candidateAllocations records every new-instance before the constructor sites
// are known. The category analysis types the uninitialized values, and
// resolveAllocationSites then finds the constructor calls that consume them.
func candidateAllocations(method *Method, file *model.File) ([]Allocation, error) {
	allocations := make([]Allocation, 0)
	for index := range method.Instructions {
		instruction := method.Instructions[index]
		if instruction.Opcode != 0x22 {
			continue
		}
		descriptor := file.Types[instruction.Index].Descriptor
		if !strings.HasPrefix(descriptor, "L") {
			return nil, diagnostic(method.Context, &instruction, "", fmt.Errorf("new-instance type %q is not an object", descriptor))
		}
		allocation := Allocation{
			ID: instruction.Offset + 1, Descriptor: descriptor, NewInstruction: index,
		}
		// Seed the first constructor site with a linear scan so the first
		// category pass already retires the uninitialized aliases. The
		// category-based discovery below then corrects and extends the set.
		if site, aliases, fused := seedAllocationSite(method, file, index); site >= 0 {
			allocation.Constructors = []int{site}
			allocation.Constructor = site
			allocation.AliasRegisters = aliases
			allocation.Fused = fused
		}
		allocations = append(allocations, allocation)
	}
	return allocations, nil
}

// seedAllocationSite finds the first constructor call with a linear scan that
// follows alias moves. It is only a seed for the first category pass; the
// category-based discovery is authoritative.
func seedAllocationSite(method *Method, file *model.File, newIndex int) (site int, aliases []uint16, fused bool) {
	registers := map[uint16]bool{method.Instructions[newIndex].Regs[0]: true}
	interleaved := false
	for index := newIndex + 1; index < len(method.Instructions); index++ {
		instruction := method.Instructions[index]
		if isDirectInvoke(instruction.Opcode) && len(instruction.Regs) > 0 && registers[instruction.Regs[0]] {
			if file.Methods[instruction.Index].Name == "<init>" {
				return index, sortedRegisters(registers), !interleaved
			}
		}
		if instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09 && registers[instruction.Regs[1]] {
			registers[instruction.Regs[0]] = true
			continue
		}
		uses, definitions := method.EffectsAt(index)
		for _, register := range uses {
			if registers[register] {
				return -1, nil, false
			}
		}
		for _, register := range definitions {
			delete(registers, register)
		}
		if len(registers) == 0 {
			return -1, nil, false
		}
		interleaved = true
	}
	return -1, nil, false
}

// resolveAllocationSites derives each allocation's constructor sites from the
// register categories: a direct constructor call whose receiver still holds an
// uninitialized value of that allocation. This is path-sensitive, so a
// conditional constructor call or a branch that reuses a register cannot
// confuse the pairing. It reports whether anything changed.
func (method *Method) indexAllocations() {
	method.allocationNew = make(map[int]bool, len(method.Allocations))
	method.allocationCtor = make(map[int]bool, len(method.Allocations))
	for index := range method.Allocations {
		allocation := &method.Allocations[index]
		method.allocationNew[allocation.NewInstruction] = true
		for _, constructor := range allocation.Constructors {
			method.allocationCtor[constructor] = true
		}
	}
}

func resolveAllocationSites(method *Method, file *model.File) (bool, error) {
	position := make(map[uint32]int, len(method.Allocations))
	for index := range method.Allocations {
		position[method.Allocations[index].ID] = index
	}
	sites := make([][]int, len(method.Allocations))
	aliases := make([]map[uint16]bool, len(method.Allocations))
	for index := range method.Instructions {
		instruction := &method.Instructions[index]
		if !isDirectInvoke(instruction.Opcode) || len(instruction.Regs) == 0 {
			continue
		}
		if file.Methods[instruction.Index].Name != "<init>" {
			continue
		}
		receiver := method.CategoryAt(index, instruction.Regs[0])
		if receiver.Kind != Uninitialized {
			continue
		}
		allocation, ok := position[receiver.Allocation]
		if !ok {
			continue
		}
		sites[allocation] = append(sites[allocation], index)
		if aliases[allocation] == nil {
			aliases[allocation] = uninitializedRegisters(method, index, receiver.Allocation)
		}
	}
	changed := false
	for index := range method.Allocations {
		allocation := &method.Allocations[index]
		aliasRegisters := sortedRegisters(aliases[index])
		fused := len(sites[index]) == 1 && fusedAllocationShape(method, allocation, sites[index][0])
		if !sameInstructionList(allocation.Constructors, sites[index]) || !sameRegisters(allocation.AliasRegisters, aliasRegisters) || allocation.Fused != fused {
			allocation.Constructors = sites[index]
			if len(sites[index]) > 0 {
				allocation.Constructor = sites[index][0]
			}
			allocation.AliasRegisters = aliasRegisters
			allocation.Fused = fused
			changed = true
		}
	}
	return changed, nil
}

// uninitializedRegisters lists the registers that hold this allocation at an
// instruction.
func uninitializedRegisters(method *Method, index int, allocation uint32) map[uint16]bool {
	registers := make(map[uint16]bool)
	for register := 0; register < int(method.Code.RegistersSize); register++ {
		category := method.CategoryAt(index, uint16(register))
		if category.Kind == Uninitialized && category.Allocation == allocation {
			registers[uint16(register)] = true
		}
	}
	return registers
}

// fusedAllocationShape reports whether the allocation and its single
// constructor call share a block with only alias moves in between.
func fusedAllocationShape(method *Method, allocation *Allocation, constructor int) bool {
	if blockOf(method, allocation.NewInstruction) != blockOf(method, constructor) {
		return false
	}
	for index := allocation.NewInstruction + 1; index < constructor; index++ {
		instruction := method.Instructions[index]
		if instruction.Opcode < 0x01 || instruction.Opcode > 0x09 {
			return false
		}
		source := method.CategoryAt(index, instruction.Regs[1])
		if source.Kind != Uninitialized || source.Allocation != allocation.ID {
			return false
		}
	}
	return true
}

func blockOf(method *Method, instruction int) int {
	if method == nil || instruction < 0 || instruction >= len(method.blockByInstruction) {
		return -1
	}
	return method.blockByInstruction[instruction]
}

func sameInstructionList(left, right []int) bool {
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

func sameRegisters(left, right []uint16) bool {
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

// validateAllocationSites checks the converged allocation model: every
// allocation is initialized, every constructor matches its type, and no
// instruction reads an uninitialized value except through an alias move or the
// constructor that consumes it.
func validateAllocationSites(method *Method, file *model.File) error {
	// Report the reading instruction first: an uninitialized value that escapes
	// is more precise than the missing constructor call.
	for index := range method.Instructions {
		instruction := &method.Instructions[index]
		uses, _ := method.EffectsAt(index)
		for _, register := range uses {
			if method.CategoryAt(index, register).Kind != Uninitialized {
				continue
			}
			if isDirectInvoke(instruction.Opcode) && len(instruction.Regs) > 0 && instruction.Regs[0] == register {
				continue
			}
			if instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09 && instruction.Regs[1] == register {
				continue
			}
			return diagnostic(method.Context, instruction, "", fmt.Errorf("unsafe uninitialized allocation use or escape before constructor"))
		}
	}
	for index := range method.Allocations {
		allocation := &method.Allocations[index]
		instruction := &method.Instructions[allocation.NewInstruction]
		// An allocation may be dead (for example an API outline that overwrites
		// the value immediately). The use scan above already rejects any read of
		// an uninitialized value; an unreferenced allocation needs no constructor.
		_ = instruction
		for _, site := range allocation.Constructors {
			constructor := &method.Instructions[site]
			callee := file.Methods[constructor.Index]
			if callee.Name != "<init>" || callee.Class != allocation.Descriptor {
				return diagnostic(method.Context, constructor, "", fmt.Errorf("allocation must be initialized by %s.<init>", allocation.Descriptor))
			}
			for _, argument := range constructor.Regs[1:] {
				category := method.CategoryAt(site, argument)
				if category.Kind == Uninitialized && category.Allocation == allocation.ID {
					return diagnostic(method.Context, constructor, "", fmt.Errorf("uninitialized allocation is also a constructor argument"))
				}
			}
		}
	}
	return nil
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
	// The second register of a category-2 value is not a value of its own. When
	// a join pairs it with a real category, the wide value died on that path and
	// the real category wins.
	if left.Kind == WideUpper || right.Kind == WideUpper {
		other := left
		if left.Kind == WideUpper {
			other = right
		}
		if other.Kind == WideUpper || other.Kind == Unknown {
			return Category{Kind: WideUpper}, left.Kind != WideUpper, nil
		}
		return other, left.Kind == WideUpper, nil
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
		if typed.Kind == Int || typed.Kind == Float || typed.Kind == Reference || typed.Kind == Uninitialized {
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

// exceptionTypes maps every handler address to the catch type that dispatches
// to it. A typed handler wins over a catch-all sharing the same entry.
func exceptionTypes(code *model.Code) map[uint32]string {
	types := make(map[uint32]string)
	for _, try := range code.Tries {
		for _, handler := range try.Handler.Handlers {
			if _, ok := types[handler.Address]; !ok {
				types[handler.Address] = handler.Type
			}
		}
		if try.Handler.CatchAllAddress != nil {
			if _, ok := types[*try.Handler.CatchAllAddress]; !ok {
				types[*try.Handler.CatchAllAddress] = "Ljava/lang/Throwable;"
			}
		}
	}
	return types
}

// ExceptionType returns the declared catch type of a handler address, or
// java.lang.Throwable when the handler is a catch-all or unknown.
func (m *Method) ExceptionType(address uint32) string {
	if descriptor, ok := m.exceptionTypes[address]; ok && descriptor != "" {
		return descriptor
	}
	return "Ljava/lang/Throwable;"
}
