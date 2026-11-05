package ir

import (
	"fmt"
	"sort"
)

func isGoto(opcode byte) bool { return opcode >= 0x28 && opcode <= 0x2a }
func isIf(opcode byte) bool   { return opcode >= 0x32 && opcode <= 0x3d }
func isBranch(opcode byte) bool {
	return isGoto(opcode) || isIf(opcode)
}
func isTerminal(opcode byte) bool {
	return opcode >= 0x0e && opcode <= 0x11 || opcode == 0x27
}

func buildBlocks(instructions []Instruction, codeUnits uint32, ctx Context) ([]Block, error) {
	if len(instructions) == 0 {
		return nil, diagnostic(ctx, nil, "code", fmt.Errorf("empty instruction stream"))
	}
	byOffset := make(map[uint32]int, len(instructions))
	for index, instruction := range instructions {
		byOffset[instruction.Offset] = index
	}
	leaders := map[uint32]bool{0: true}
	for index, instruction := range instructions {
		if isBranch(instruction.Opcode) {
			if instruction.Target < 0 || instruction.Target >= int64(codeUnits) {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("branch target %d is outside code units [0,%d)", instruction.Target, codeUnits))
			}
			target := uint32(instruction.Target)
			if _, ok := byOffset[target]; !ok {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("branch target %d is not an instruction boundary", target))
			}
			if target == instruction.Offset {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("zero-offset branch is invalid"))
			}
			leaders[target] = true
			if index+1 < len(instructions) {
				leaders[instructions[index+1].Offset] = true
			}
		}
		if isTerminal(instruction.Opcode) && index+1 < len(instructions) {
			leaders[instructions[index+1].Offset] = true
		}
	}
	starts := make([]uint32, 0, len(leaders))
	for start := range leaders {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	blocks := make([]Block, len(starts))
	blockAt := make(map[uint32]int, len(starts))
	for index, start := range starts {
		end := codeUnits
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		block := Block{ID: index, Start: start, End: end}
		for instructionIndex, instruction := range instructions {
			if instruction.Offset >= start && instruction.Offset < end {
				block.Instructions = append(block.Instructions, instructionIndex)
			}
		}
		if len(block.Instructions) == 0 {
			return nil, diagnostic(ctx, nil, "cfg", fmt.Errorf("block at code-unit offset %d is empty", start))
		}
		blocks[index] = block
		blockAt[start] = index
	}
	addSuccessor := func(block *Block, successor int) {
		for _, existing := range block.Successors {
			if existing == successor {
				return
			}
		}
		block.Successors = append(block.Successors, successor)
	}
	for index := range blocks {
		block := &blocks[index]
		last := instructions[block.Instructions[len(block.Instructions)-1]]
		if isBranch(last.Opcode) {
			addSuccessor(block, blockAt[uint32(last.Target)])
			if isIf(last.Opcode) && index+1 < len(blocks) {
				addSuccessor(block, index+1)
			}
		} else if !isTerminal(last.Opcode) {
			if index+1 >= len(blocks) {
				return nil, diagnostic(ctx, &last, "", fmt.Errorf("reachable execution may fall off end of code"))
			}
			addSuccessor(block, index+1)
		}
	}
	for index := range blocks {
		for _, successor := range blocks[index].Successors {
			blocks[successor].Predecessors = append(blocks[successor].Predecessors, index)
		}
	}
	return blocks, nil
}
