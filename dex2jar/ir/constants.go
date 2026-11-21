package ir

import (
	"fmt"

	"github.com/yaklang/javajive/dex2jar/model"
)

type demandState struct {
	single Category
	mask   uint8
}

func zeroMaskBit(kind CategoryKind) uint8 {
	switch kind {
	case Int:
		return PolyZeroInt
	case Float:
		return PolyZeroFloat
	case Reference:
		return PolyZeroRef
	default:
		return 0
	}
}

func (d demandState) isPolyZero() bool {
	return d.mask&(d.mask-1) != 0
}

func demandSlicesEqual(left, right []demandState) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// refineConstants propagates the categories demanded at use sites backwards to
// constant definitions. DEX constants carry raw bits, so only this backward
// pass can tell a long constant from a double constant (or an int from a float,
// or a polymorphic zero constant shared across int/float/reference uses).
func refineConstants(method *Method, file *model.File) (bool, error) {
	registers := int(method.Code.RegistersSize)
	demandIn := make([][]demandState, len(method.Blocks))
	for index := range demandIn {
		demandIn[index] = make([]demandState, registers)
	}
	// Exception edges carry the demand observed at the protected instruction, not
	// at the end of the protected block, so they are injected at the instruction
	// during the reverse walk instead of joined from the block exit.
	protectedHandlers := make([][]int, len(method.Instructions))
	blockOf := make([]int, len(method.Instructions))
	for blockIndex := range method.Blocks {
		for _, instructionIndex := range method.Blocks[blockIndex].Instructions {
			blockOf[instructionIndex] = blockIndex
		}
	}
	exceptionEdge := make([]map[int]bool, len(method.Blocks))
	for handler := range method.Blocks {
		for _, instructionIndex := range method.Blocks[handler].Protected {
			protectedHandlers[instructionIndex] = append(protectedHandlers[instructionIndex], handler)
			blockIndex := blockOf[instructionIndex]
			if exceptionEdge[blockIndex] == nil {
				exceptionEdge[blockIndex] = make(map[int]bool)
			}
			exceptionEdge[blockIndex][handler] = true
		}
	}
	changed := false
	polyMasks := make([]uint8, registers)
	anyPoly := false
	for pass := 0; pass <= len(method.Blocks)+1; pass++ {
		iteration := false
		polyMasks = make([]uint8, registers)
		anyPoly = false
		for blockIndex := len(method.Blocks) - 1; blockIndex >= 0; blockIndex-- {
			// The demand leaving a block is the join of every successor entry.
			next := make([]demandState, registers)
			for _, successor := range method.Blocks[blockIndex].Successors {
				if exceptionEdge[blockIndex][successor] {
					continue
				}
				for register := 0; register < registers; register++ {
					if err := joinDemandState(&next[register], demandIn[successor][register]); err != nil {
						instruction := method.Instructions[firstInstruction(method, blockIndex)]
						return false, diagnostic(method.Context, &instruction, "", fmt.Errorf("v%d has incompatible demanded categories: %w", register, err))
					}
				}
			}
			demands := append([]demandState(nil), next...)
			block := method.Blocks[blockIndex]
			for offset := len(block.Instructions) - 1; offset >= 0; offset-- {
				index := block.Instructions[offset]
				instruction := &method.Instructions[index]
				definition, defined := method.DefinitionAt(index)
				var carry demandState
				if defined {
					carry = demands[definition.Register]
					// The old value of the destination dies here; operands below
					// still refer to it and are added again after the kill.
					demands[definition.Register] = demandState{}
				}
				if instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09 {
					// A move produces the same value as its source, so the demand
					// on the destination is a demand on the source.
					if err := joinDemandState(&demands[instruction.Regs[1]], carry); err != nil {
						return false, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d: %w", instruction.Regs[1], err))
					}
				} else {
					for _, operand := range method.demands[index] {
						if err := joinDemand(&demands[operand.register], operand.category); err != nil {
							return false, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d: %w", operand.register, err))
						}
					}
				}
				for _, handler := range protectedHandlers[index] {
					for register := 0; register < registers; register++ {
						demand := demandIn[handler][register]
						if demand.single.Kind == Unknown {
							continue
						}
						if err := joinDemandState(&demands[register], demand); err != nil {
							return false, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d: %w", register, err))
						}
					}
				}
				if defined && carry.single.Kind != Unknown {
					if carry.isPolyZero() {
						isConstZero := instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15 && instruction.Literal == 0
						isPlainMoveOfNull := instruction.Opcode >= 0x01 && instruction.Opcode <= 0x03 && definition.Category.Kind == Null
						if !isConstZero && !isPlainMoveOfNull {
							return false, diagnostic(method.Context, instruction, "", fmt.Errorf("v%d has incompatible demanded categories", definition.Register))
						}
						polyMasks[definition.Register] |= carry.mask
						anyPoly = true
						if isConstZero {
							resolved := Category{Kind: Null, Allocation: PolyZeroAllocation}
							if resolved != definition.Category {
								if method.constantKinds == nil {
									method.constantKinds = make(map[int]Category)
								}
								if previous, ok := method.constantKinds[index]; !ok || previous != resolved {
									method.constantKinds[index] = resolved
									changed = true
								}
							}
						}
					} else if definition.Category.Kind.isUntyped() {
						if definition.Category.Kind == Null && definition.Category.Allocation == PolyZeroAllocation && carry.mask != 0 {
							polyMasks[definition.Register] |= carry.mask
							anyPoly = true
						} else {
							resolved, err := resolveConstant(definition.Category, carry.single)
							if err != nil {
								return false, diagnostic(method.Context, instruction, "", err)
							}
							if instruction.Opcode >= 0x12 && instruction.Opcode <= 0x19 && resolved != definition.Category {
								if method.constantKinds == nil {
									method.constantKinds = make(map[int]Category)
								}
								if previous, ok := method.constantKinds[index]; !ok || previous != resolved {
									method.constantKinds[index] = resolved
									changed = true
								}
							}
						}
					}
				}
			}
			if !demandSlicesEqual(demandIn[blockIndex], demands) {
				demandIn[blockIndex] = demands
				iteration = true
			}
		}
		if !iteration {
			break
		}
	}
	if len(demandIn) > 0 {
		for register := 0; register < registers; register++ {
			if demandIn[0][register].isPolyZero() {
				instruction := method.Instructions[firstInstruction(method, 0)]
				return false, diagnostic(method.Context, &instruction, "", fmt.Errorf("v%d has incompatible demanded categories", register))
			}
		}
	}
	if anyPoly {
		method.PolyZeroRegisters = polyMasks
	} else {
		method.PolyZeroRegisters = nil
	}
	return changed, nil
}

// joinDemand merges one required category into a demand slot.
func joinDemand(slot *demandState, category Category) error {
	if category.Kind == Unknown {
		return nil
	}
	bit := zeroMaskBit(category.Kind)
	if slot.single.Kind == Unknown {
		slot.single = category
		slot.mask = bit
		return nil
	}
	if slot.isPolyZero() {
		if bit != 0 {
			slot.mask |= bit
			return nil
		}
		if category.Kind == Null {
			return nil
		}
		return fmt.Errorf("incompatible constant categories %s and %s", slot.single.Kind, category.Kind)
	}
	if bit != 0 && slot.mask != 0 && slot.single.Kind != category.Kind {
		slot.mask |= bit
		slot.single = Category{Kind: Null, Allocation: PolyZeroAllocation}
		return nil
	}
	merged, _, err := mergeCategory(slot.single, category)
	if err != nil {
		return err
	}
	slot.single = merged
	if mergedBit := zeroMaskBit(merged.Kind); mergedBit != 0 {
		slot.mask |= mergedBit
	}
	return nil
}

func joinDemandState(slot *demandState, incoming demandState) error {
	if incoming.single.Kind == Unknown {
		return nil
	}
	if !incoming.isPolyZero() {
		return joinDemand(slot, incoming.single)
	}
	if slot.single.Kind == Unknown || slot.single.Kind == Null {
		*slot = demandState{
			single: Category{Kind: Null, Allocation: PolyZeroAllocation},
			mask:   slot.mask | incoming.mask,
		}
		return nil
	}
	if slot.mask != 0 {
		slot.mask |= incoming.mask
		slot.single = Category{Kind: Null, Allocation: PolyZeroAllocation}
		return nil
	}
	return fmt.Errorf("incompatible constant categories %s and %s", slot.single.Kind, incoming.single.Kind)
}

// resolveConstant turns an open constant category into the category its uses
// demand. A constant that is never typed keeps its raw width.
func resolveConstant(category, demand Category) (Category, error) {
	switch category.Kind {
	case Null:
		switch demand.Kind {
		case Int, Float, Reference:
			// The zero constant is 32 bits and fits all three readings.
			return demand, nil
		case Bits32:
			// A move chain left the interpretation open; the raw zero bits are
			// an int constant.
			return Category{Kind: Int}, nil
		case Unknown, Null:
			return category, nil
		}
	case Bits32:
		switch demand.Kind {
		case Int, Float:
			return demand, nil
		case Unknown, Bits32:
			return category, nil
		}
	case Bits64:
		switch demand.Kind {
		case Long, Double:
			return demand, nil
		case Unknown, Bits64:
			return category, nil
		}
	}
	return Category{}, fmt.Errorf("constant of width %s is used as %s", category.Kind, demand.Kind)
}

// constantCategory returns the category a constant instruction defines. The
// width is fixed by the opcode; constantKinds records the interpretation the
// backward refinement derived from use sites.
func (m *Method) constantCategory(index int, fallback Category) Category {
	if refined, ok := m.constantKinds[index]; ok {
		return refined
	}
	return fallback
}
