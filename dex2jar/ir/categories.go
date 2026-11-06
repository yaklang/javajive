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
// definite-assignment and category analysis. A category-2 definition also
// defines the following register, which is how the pair stays assigned.
func instructionEffects(instruction Instruction) (uses, defs []uint16) {
	registers := instruction.Regs
	switch {
	case instruction.Opcode == 0x00 || instruction.Opcode == 0x0e || instruction.Opcode >= 0x28 && instruction.Opcode <= 0x2a:
		return nil, nil
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09:
		uses, defs = []uint16{registers[1]}, []uint16{registers[0]}
	case instruction.Opcode >= 0x0a && instruction.Opcode <= 0x0d:
		// move-result and move-exception store an already-produced value.
		defs = []uint16{registers[0]}
	case instruction.Opcode == 0x0f || instruction.Opcode == 0x10 || instruction.Opcode == 0x11:
		uses = []uint16{registers[0]}
	case instruction.Opcode == 0x1d || instruction.Opcode == 0x1e || instruction.Opcode == 0x27:
		uses = []uint16{registers[0]}
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x1c || instruction.Opcode == 0x22:
		defs = []uint16{registers[0]}
	case instruction.Opcode == 0x1f:
		uses, defs = []uint16{registers[0]}, []uint16{registers[0]}
	case instruction.Opcode == 0x20 || instruction.Opcode == 0x21 || instruction.Opcode == 0x23:
		uses, defs = []uint16{registers[1]}, []uint16{registers[0]}
	case instruction.Opcode == 0x24 || instruction.Opcode == 0x25:
		uses = append([]uint16(nil), registers...)
	case instruction.Opcode == 0x26:
		uses = []uint16{registers[0]}
	case instruction.Opcode == 0x2b || instruction.Opcode == 0x2c:
		uses = []uint16{registers[0]}
	case instruction.Opcode >= 0x2d && instruction.Opcode <= 0x31:
		uses, defs = []uint16{registers[1], registers[2]}, []uint16{registers[0]}
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x37:
		uses = append([]uint16(nil), registers...)
	case instruction.Opcode >= 0x38 && instruction.Opcode <= 0x3d:
		uses = []uint16{registers[0]}
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x4a:
		uses, defs = []uint16{registers[1], registers[2]}, []uint16{registers[0]}
	case instruction.Opcode >= 0x4b && instruction.Opcode <= 0x51:
		uses = []uint16{registers[0], registers[1], registers[2]}
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x58:
		uses, defs = []uint16{registers[1]}, []uint16{registers[0]}
	case instruction.Opcode >= 0x59 && instruction.Opcode <= 0x5f:
		uses = []uint16{registers[0], registers[1]}
	case instruction.Opcode >= 0x60 && instruction.Opcode <= 0x66:
		defs = []uint16{registers[0]}
	case instruction.Opcode >= 0x67 && instruction.Opcode <= 0x6d:
		uses = []uint16{registers[0]}
	case isInvoke(instruction.Opcode):
		uses = append([]uint16(nil), registers...)
	case instruction.Opcode >= 0x7b && instruction.Opcode <= 0x8f:
		uses, defs = []uint16{registers[1]}, []uint16{registers[0]}
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0xaf:
		uses, defs = []uint16{registers[1], registers[2]}, []uint16{registers[0]}
	case instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xcf:
		uses, defs = []uint16{registers[0], registers[1]}, []uint16{registers[0]}
	case instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		uses, defs = []uint16{registers[1]}, []uint16{registers[0]}
	default:
		return append([]uint16(nil), registers...), nil
	}
	if wideDefinitionOpcode(instruction.Opcode) && len(defs) == 1 {
		defs = append(defs, defs[0]+1)
	}
	return uses, defs
}

// wideDefinitionOpcode reports whether the destination of an opcode is a
// category-2 value that occupies the following register too.
func wideDefinitionOpcode(opcode byte) bool {
	switch {
	case opcode >= 0x04 && opcode <= 0x06, opcode == 0x0b, opcode >= 0x16 && opcode <= 0x19:
		return true
	case opcode == 0x45, opcode == 0x53, opcode == 0x61:
		return true
	case opcode == 0x7d || opcode == 0x7e || opcode == 0x80:
		return true
	case opcode == 0x81 || opcode == 0x83 || opcode == 0x86 || opcode == 0x88 || opcode == 0x89 || opcode == 0x8b:
		return true
	case opcode >= 0x9b && opcode <= 0xa5 || opcode >= 0xab && opcode <= 0xaf:
		return true
	case opcode >= 0xbb && opcode <= 0xc5 || opcode >= 0xcb && opcode <= 0xcf:
		return true
	default:
		return false
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

// demand is one concrete category an instruction requires of an operand. It is
// recorded while walking a block and consumed by the backward refinement pass.
type demand struct {
	register uint16
	category Category
}

// blockRun is the local result of walking one basic block.
type blockRun struct {
	states      [][]Category
	definitions []Definition
	demands     [][]demand
	exit        []Category
}

// walkBlock applies the instruction category constraints in program order.
// Definitions replace the previous lifetime of a register; uses must agree with
// the category already established for that register.
func walkBlock(method *Method, file *model.File, allocations []Allocation, blockIndex int, entryState []Category, final bool) (*blockRun, error) {
	block := method.Blocks[blockIndex]
	run := &blockRun{
		states:      make([][]Category, 0, len(block.Instructions)),
		definitions: make([]Definition, 0, len(block.Instructions)),
		demands:     make([][]demand, 0, len(block.Instructions)),
	}
	state := append([]Category(nil), entryState...)
	// A constant has no intrinsic type; it adopts the type of its first use.
	// Track where the current untyped lifetime started so the definition and
	// every intermediate state can be refined retroactively.
	untypedLifetime := make(map[uint16]int)
	wideHigh := func(register uint16, instruction *Instruction) error {
		if int(register)+1 >= len(state) {
			return diagnostic(method.Context, instruction, "", fmt.Errorf("wide value in v%d requires register v%d", register, register+1))
		}
		state[register+1] = Category{Kind: WideUpper}
		delete(untypedLifetime, register+1)
		return nil
	}
	for _, instructionIndex := range block.Instructions {
		run.states = append(run.states, append([]Category(nil), state...))
		position := len(run.states) - 1
		instruction := &method.Instructions[instructionIndex]
		uses, defs, err := instructionConstraints(method, file, instructionIndex, state, allocations, final)
		if err != nil {
			return nil, diagnostic(method.Context, instruction, "", err)
		}
		record := make([]demand, 0, len(uses))
		// Operand requirements first: they constrain the values read at this
		// point, including a destination register that is also an operand.
		for _, register := range sortedCategoryRegisters(uses) {
			category := uses[register]
			if category.Kind == Unknown {
				continue
			}
			record = append(record, demand{register: register, category: category})
			merged, _, err := mergeCategory(state[register], category)
			if err != nil {
				return nil, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d: %w", register, err))
			}
			if state[register].Kind.isUntyped() && merged.Kind != state[register].Kind {
				if start, ok := untypedLifetime[register]; ok && start < position {
					for index := start + 1; index <= position; index++ {
						run.states[index][register] = merged
					}
					run.definitions[start].Category = merged
					delete(untypedLifetime, register)
				}
			}
			state[register] = merged
			if merged.Kind.Words() == 2 {
				if err := wideHigh(register, instruction); err != nil {
					return nil, err
				}
			}
		}
		run.demands = append(run.demands, record)
		definition := Definition{Register: -1}
		for _, register := range sortedCategoryRegisters(defs) {
			category := defs[register]
			if category.Kind == Unknown {
				continue
			}
			state[register] = category
			definition = Definition{Register: int32(register), Category: category}
			if category.Kind.isUntyped() {
				untypedLifetime[register] = position
			} else {
				delete(untypedLifetime, register)
			}
			if category.Kind.Words() == 2 {
				if err := wideHigh(register, instruction); err != nil {
					return nil, err
				}
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
		words := kind.Words()
		// A category-2 parameter occupies its register and the next one.
		if words == 2 && parameter+1 < registerCount {
			entry[parameter+1] = Category{Kind: WideUpper}
		}
		parameter += words
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

	// Constants are raw bit patterns, so a const instruction only knows its
	// width; the backward refinement pass resolves its interpretation and the
	// forward solution is recomputed until no constant changes kind.
	for pass := 0; pass <= len(method.Instructions)+2; pass++ {
		if _, err := solveCategories(method, file, allocations, entry, registerCount, propagate); err != nil {
			return nil, err
		}
		changed, err := refineConstants(method, file)
		if err != nil {
			return nil, err
		}
		if !changed {
			return entry, nil
		}
	}
	return nil, diagnostic(method.Context, nil, "categories", fmt.Errorf("constant category inference did not converge"))
}

// solveCategories runs the forward category fixpoint and materializes the
// per-instruction states, definitions and operand demands.
func solveCategories(method *Method, file *model.File, allocations []Allocation, entry []Category, registerCount int, propagate []map[uint16]bool) ([][]Category, error) {
	protected := make([]map[int]bool, len(method.Blocks))
	for index := range method.Blocks {
		if len(method.Blocks[index].Protected) == 0 {
			continue
		}
		set := make(map[int]bool, len(method.Blocks[index].Protected))
		for _, instructionIndex := range method.Blocks[index].Protected {
			set[instructionIndex] = true
		}
		protected[index] = set
	}
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
					if !method.Blocks[predecessor].Reachable {
						// Dead code may fall through into a live block; its register
						// state is not part of any real path.
						continue
					}
					predecessorRun, err := walkBlock(method, file, allocations, predecessor, entries[predecessor], false)
					if err != nil {
						return nil, err
					}
					incoming := predecessorRun.exit[register]
					if protected[blockIndex] != nil {
						// A handler observes the register file as it was when the
						// protected instruction started, so use those pre-states
						// instead of the protected block's exit state.
						replacement, ok, err := protectedState(method, predecessorRun, predecessor, protected[blockIndex], register)
						if err != nil {
							instruction := &method.Instructions[firstInstruction(method, blockIndex)]
							return nil, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d has incompatible exception states: %w", register, err))
						}
						if ok {
							incoming = replacement
						}
					}
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

	// Materialize per-instruction states, definitions and demands.
	stride := registerCount
	method.CategoryStride = stride
	method.Categories = make([]Category, len(method.Instructions)*stride)
	method.Definitions = make([]Definition, len(method.Instructions))
	method.demands = make([][]demand, len(method.Instructions))
	for index := range method.Definitions {
		method.Definitions[index].Register = -1
	}
	for blockIndex, block := range method.Blocks {
		run, err := walkBlock(method, file, allocations, blockIndex, entries[blockIndex], method.Blocks[blockIndex].Reachable)
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
			method.demands[instructionIndex] = run.demands[offset]
		}
	}
	return entries, nil
}

// computeLiveness runs the standard backward live-variable analysis over the
// block graph using the per-instruction use/define table.
// protectedState joins the pre-instruction categories of the protected
// instructions that live in one predecessor block.
func protectedState(method *Method, run *blockRun, predecessor int, protected map[int]bool, register uint16) (Category, bool, error) {
	merged := Category{}
	found := false
	for offset, instructionIndex := range method.Blocks[predecessor].Instructions {
		if !protected[instructionIndex] {
			continue
		}
		incoming := run.states[offset][register]
		if incoming.Kind == Unknown {
			continue
		}
		result, _, err := mergeCategory(merged, incoming)
		if err != nil {
			return Category{}, false, err
		}
		merged = result
		found = true
	}
	return merged, found, nil
}

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
